package firefoxstorage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/snappy"
	_ "github.com/mattn/go-sqlite3"
)

// The tables of data.sqlite, trimmed to the columns the reader uses.
const localStorageTables = `
CREATE TABLE database (origin TEXT NOT NULL);
CREATE TABLE data (key TEXT PRIMARY KEY, conversion_type INTEGER NOT NULL, compression_type INTEGER NOT NULL, value BLOB NOT NULL);`

const (
	chatGPT     = "https://chatgpt.com"
	draftsKey   = "oai/apps/conversationDrafts"
	draftsJSON  = `{"drafts":[{"id":"c1","content":"olá, rascunho","timestamp":1790941431821}]}`
	themeKey    = "theme"
	unicodeText = "ação ✓"
	brokenKey   = "broken"
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

// writeLocalStorage lays out origin's ls directory: a Snappy-compressed
// JSON item, a UTF-16 one, one with an unknown compression.
func writeLocalStorage(t *testing.T, origin string) string {
	t.Helper()
	location := filepath.Join(t.TempDir(), "https+++chatgpt.com", localStorageDir)
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	db, err := openForTest(filepath.Join(location, localStorageDatabase))
	testcheck.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(localStorageTables)
	testcheck.NoError(t, err)
	insert := "INSERT INTO data VALUES (?, ?, ?, ?)"
	_, err = db.Exec("INSERT INTO database VALUES (?)", origin)
	testcheck.NoError(t, err)
	_, err = db.Exec(insert, draftsKey, conversionUTF8, compressionSnap, snappy.Encode(nil, []byte(draftsJSON)))
	testcheck.NoError(t, err)
	_, err = db.Exec(insert, themeKey, conversionNone, compressionNone, utf16LE(unicodeText))
	testcheck.NoError(t, err)
	_, err = db.Exec(insert, brokenKey, conversionUTF8, 9, []byte("x"))
	testcheck.NoError(t, err)
	return location
}

func readerOf(t *testing.T, origins ...string) LocalStorageReader {
	t.Helper()
	scope, err := sitestorage.NewScope(origins)
	testcheck.NoError(t, err)
	return NewLocalStorageReader(scope, openForTest)
}

func recordOf(t *testing.T, records []webstore.Record, container string) webstore.Record {
	t.Helper()
	for _, record := range records {
		if record.Container == container {
			return record
		}
	}
	t.Fatalf("no record in container %q among %+v", container, records)
	return webstore.Record{}
}

func TestReadDecodesSnappyJSONItems(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t, chatGPT))
	testcheck.NoError(t, err)
	drafts := recordOf(t, records, draftsKey)
	if drafts.Origin != chatGPT || drafts.Key != draftsKey || drafts.Value.Get("drafts").Items[0].Get("content").String() != "olá, rascunho" {
		t.Fatalf("drafts record = %+v; want the parsed draft of ChatGPT", drafts)
	}
}

func TestReadDecodesUnconvertedUTF16Items(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t, chatGPT))
	testcheck.NoError(t, err)
	if theme := recordOf(t, records, themeKey); theme.Value.Text != unicodeText {
		t.Fatalf("theme record = %+v; want the text %q", theme, unicodeText)
	}
}

func TestReadReportsAnUndecodableItem(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t, chatGPT))
	testcheck.NoError(t, err)
	broken := recordOf(t, records, brokenKey)
	if broken.DecodeErr == nil || !strings.Contains(broken.DecodeErr.Error(), "compression type 9") {
		t.Fatalf("broken record = %+v; want a DecodeErr naming the compression", broken)
	}
}

func TestReadRefusesAnOriginOutsideTheScope(t *testing.T) {
	location := writeLocalStorage(t, "https://teams.cloud.microsoft")
	records, err := readerOf(t, chatGPT).Read(location)
	if err == nil || records != nil || !strings.Contains(err.Error(), "https://teams.cloud.microsoft") {
		t.Fatalf("Read = %+v, %v; want a refusal naming the origin", records, err)
	}
}

func TestReadRefusesAnEmptyScope(t *testing.T) {
	if _, err := readerOf(t).Read(writeLocalStorage(t, chatGPT)); !errors.Is(err, sitestorage.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestRecognizesOnlyAnLsDirectory(t *testing.T) {
	location := writeLocalStorage(t, chatGPT)
	reader := readerOf(t, chatGPT)
	if !reader.Recognizes(location) {
		t.Fatalf("Recognizes(%q) = false; want true", location)
	}
	idb := filepath.Join(filepath.Dir(location), "idb")
	testcheck.NoError(t, os.MkdirAll(idb, 0o700))
	testcheck.NoError(t, os.WriteFile(filepath.Join(idb, localStorageDatabase), nil, 0o600))
	if reader.Recognizes(idb) {
		t.Fatal("Recognizes = true for an idb directory")
	}
}
