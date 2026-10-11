package firefoxcache

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/snappy"
	_ "github.com/mattn/go-sqlite3"
)

// The tables of caches.sqlite, trimmed to the columns the reader uses.
const cacheTables = `
CREATE TABLE caches (id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT);
CREATE TABLE entries (id INTEGER NOT NULL PRIMARY KEY, request_url_no_query TEXT NOT NULL, request_url_query TEXT NOT NULL, response_body_id TEXT NULL, cache_id INTEGER NOT NULL REFERENCES caches(id));
CREATE TABLE storage (namespace INTEGER NOT NULL, key BLOB NULL, cache_id INTEGER NOT NULL REFERENCES caches(id), PRIMARY KEY(namespace, key));`

const (
	messagesBodyID = "{af9522c4-1b46-4255-8901-47c55163c927}"
	profileBodyID  = "{597154f9-b7e2-4d8d-a363-7dab42a7f92e}"
	browserBodyID  = "{634413f6-deec-42b3-bb56-55bbcb04302f}"
)

func openForTest(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func utf16LE(text string) []byte {
	var raw []byte
	for _, unit := range utf16.Encode([]rune(text)) {
		raw = append(raw, byte(unit), byte(unit>>8))
	}
	return raw
}

func snappyFramed(t *testing.T, plain string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := snappy.NewBufferedWriter(&out)
	_, _ = writer.Write([]byte(plain))
	testcheck.NoError(t, writer.Close())
	return out.Bytes()
}

// writeCacheDir lays out an origin's cache directory: two caches of the
// page, one of the browser, a body file per response.
func writeCacheDir(t *testing.T) string {
	t.Helper()
	location := t.TempDir()
	db, err := openForTest(filepath.Join(location, cachesDatabase))
	testcheck.NoError(t, err)
	defer db.Close()
	statements := []struct {
		query string
		args  []any
	}{
		{cacheTables, nil},
		{`INSERT INTO caches (id) VALUES (1), (2), (3)`, nil},
		{`INSERT INTO storage VALUES (0, ?, 1), (0, ?, 2), (1, ?, 3)`, []any{utf16LE("api-v1"), utf16LE("assets"), utf16LE("api-v1")}},
		{`INSERT INTO entries VALUES (1, 'https://discord.com/api/v9/channels/42/messages', '?limit=50', ?, 1), (2, 'https://discord.com/api/v9/users/@me/profile', '', ?, 2), (3, 'https://discord.com/api/v9/channels/42/messages', '', ?, 3)`,
			[]any{messagesBodyID, profileBodyID, browserBodyID}},
	}
	for _, statement := range statements {
		_, err := db.Exec(statement.query, statement.args...)
		testcheck.NoError(t, err)
	}
	writeBody(t, location, "39", messagesBodyID, snappyFramed(t, messagesBody))
	writeBody(t, location, "46", profileBodyID, snappyFramed(t, `{"bio":"x"}`))
	writeBody(t, location, "47", browserBodyID, snappyFramed(t, messagesBody))
	return location
}

func writeBody(t *testing.T, location, dir, bodyID string, framed []byte) {
	t.Helper()
	testcheck.NoError(t, os.MkdirAll(filepath.Join(location, morgueDir, dir), 0o755))
	testcheck.NoError(t, os.WriteFile(filepath.Join(location, morgueDir, dir, bodyID+bodySuffix), framed, 0o600))
}

func TestCacheAPIReadReturnsTheResponsesInScope(t *testing.T) {
	records, err := NewCacheAPIReader(discordScope(t), openForTest).Read(writeCacheDir(t))
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %+v, %v; want only the page's messages response", records, err)
	}
	record := records[0]
	if record.Kind != webstore.KindCacheAPI || record.Namespace != "api-v1" || record.Container != scopeName || record.Key != messagesURL {
		t.Fatalf("record = %+v; want the cache name, scope name and URL with its query", record)
	}
	if record.DecodeErr != nil || record.Value.Items[0].Get("content").String() != "olá" {
		t.Fatalf("record value = %+v, %v; want the decoded message", record.Value, record.DecodeErr)
	}
}

// RecordingFileReader reads body files and remembers their names.
type RecordingFileReader struct {
	names []string
}

func (r *RecordingFileReader) ReadFile(path string) ([]byte, error) {
	r.names = append(r.names, filepath.Base(path))
	return os.ReadFile(path)
}

func TestCacheAPIReadNeverReadsTheBodyOfAURLOutOfScope(t *testing.T) {
	files := &RecordingFileReader{}
	reader := NewCacheAPIReader(discordScope(t), openForTest)
	reader.readFile = files.ReadFile
	if _, err := reader.Read(writeCacheDir(t)); err != nil {
		t.Fatal(err)
	}
	if len(files.names) != 1 || files.names[0] != messagesBodyID+bodySuffix {
		t.Fatalf("body files read = %v; want only the one in scope", files.names)
	}
}

func TestCacheAPIReadKeepsAMissingBodyAsARecord(t *testing.T) {
	location := writeCacheDir(t)
	testcheck.NoError(t, os.RemoveAll(filepath.Join(location, morgueDir, "39")))
	records, err := NewCacheAPIReader(discordScope(t), openForTest).Read(location)
	if err != nil || len(records) != 1 || records[0].DecodeErr == nil || records[0].Value != nil {
		t.Fatalf("Read = %+v, %v; want the response counted with DecodeErr", records, err)
	}
}

func TestReadBodyFileRefusesAnIDThatIsNotAUUID(t *testing.T) {
	_, err := NewCacheAPIReader(discordScope(t), openForTest).readBodyFile(t.TempDir(), "../../etc/passwd")
	if err == nil || !strings.Contains(err.Error(), `"../../etc/passwd"`) {
		t.Fatalf("readBodyFile error = %v; want the id refused by value", err)
	}
}

func TestCacheAPIReadRefusesAnEmptyScope(t *testing.T) {
	if _, err := NewCacheAPIReader(requestcache.Scope{}, openForTest).Read(writeCacheDir(t)); !errors.Is(err, requestcache.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestCacheAPIRecognizesAnOriginCacheDirectory(t *testing.T) {
	reader := NewCacheAPIReader(discordScope(t), openForTest)
	if !reader.Recognizes(writeCacheDir(t)) || reader.Recognizes(writeCache2(t, nil)) {
		t.Fatal("Recognizes should accept a cache directory and refuse cache2")
	}
}
