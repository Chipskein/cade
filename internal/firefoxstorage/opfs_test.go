package firefoxstorage

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/webstore"
)

// The tables of metadata.sqlite, trimmed to the columns the reader uses.
const opfsTables = `
CREATE TABLE Entries (handle BLOB PRIMARY KEY, parent BLOB);
CREATE TABLE Directories (handle BLOB PRIMARY KEY, name BLOB NOT NULL);
CREATE TABLE Files (handle BLOB PRIMARY KEY, type TEXT, name BLOB NOT NULL);
CREATE TABLE MainFiles (handle BLOB UNIQUE, fileId BLOB UNIQUE);`

const (
	notion      = "https://app.notion.com"
	notionDir   = "https+++app.notion.com"
	chatJSON    = `{"messages":[{"id":"m1","text":"oi"}]}`
	stateJSON   = `{"open":"c1"}`
	chatPath    = "cache/chats/c1.json"
	statePath   = "state.json"
	sqlitePath  = "cache/db.sqlite3"
	pendingFile = "pending.json"
)

// opfsLayout writes entries and the bytes of files into an fs directory.
type opfsLayout struct {
	t        *testing.T
	db       *sql.DB
	location string
}

func newOPFSLayout(t *testing.T, originDir string) opfsLayout {
	t.Helper()
	location := filepath.Join(t.TempDir(), originDir, opfsDir)
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	db, err := openForTest(filepath.Join(location, opfsMetadata))
	testcheck.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(opfsTables)
	testcheck.NoError(t, err)
	layout := opfsLayout{t: t, db: db, location: location}
	layout.exec("INSERT INTO Entries VALUES ('root', NULL)")
	layout.exec("INSERT INTO Directories VALUES ('root', ?)", utf16LE("root"))
	return layout
}

func (l opfsLayout) exec(query string, args ...any) {
	l.t.Helper()
	_, err := l.db.Exec(query, args...)
	testcheck.NoError(l.t, err)
}

func (l opfsLayout) dir(handle, parent, name string) {
	l.exec("INSERT INTO Entries VALUES (?, ?)", handle, parent)
	l.exec("INSERT INTO Directories VALUES (?, ?)", handle, utf16LE(name))
}

// file adds a file entry; content nil leaves it without bytes, as a file
// created and not yet written.
func (l opfsLayout) file(handle, parent, name string, content []byte) {
	l.exec("INSERT INTO Entries VALUES (?, ?)", handle, parent)
	l.exec("INSERT INTO Files VALUES (?, NULL, ?)", handle, utf16LE(name))
	if content == nil {
		return
	}
	fileID := []byte("id-" + handle)
	l.exec("INSERT INTO MainFiles VALUES (?, ?)", handle, fileID)
	diskName := fileIDEncoding.EncodeToString(fileID)
	dir := filepath.Join(l.location, diskName[:fileDirNameLen])
	testcheck.NoError(l.t, os.MkdirAll(dir, 0o700))
	testcheck.NoError(l.t, os.WriteFile(filepath.Join(dir, diskName), content, 0o600))
}

func writeOPFS(t *testing.T, originDir string) string {
	t.Helper()
	layout := newOPFSLayout(t, originDir)
	layout.dir("cache", "root", "cache")
	layout.dir("chats", "cache", "chats")
	layout.file("c1", "chats", "c1.json", []byte(chatJSON))
	layout.file("state", "root", statePath, []byte(stateJSON))
	layout.file("db", "cache", "db.sqlite3", []byte("SQLite format 3\x00"))
	layout.file("pending", "root", pendingFile, nil)
	layout.file("orphan", "gone", "orphan.json", []byte(stateJSON))
	return layout.location
}

func opfsReaderOf(t *testing.T, origins ...string) OPFSReader {
	t.Helper()
	scope, err := sitestorage.NewScope(origins)
	testcheck.NoError(t, err)
	return NewOPFSReader(scope, openForTest)
}

func keysOf(records []webstore.Record) []string {
	keys := make([]string, len(records))
	for i, record := range records {
		keys[i] = record.Key
	}
	return keys
}

func TestOPFSReadGivesEveryFileWithBytesByPath(t *testing.T) {
	records, err := opfsReaderOf(t, notion).Read(writeOPFS(t, notionDir))
	testcheck.NoError(t, err)
	if got := strings.Join(keysOf(records), ","); got != chatPath+","+sqlitePath+","+statePath {
		t.Fatalf("Read keys = %s; want the three reachable files with bytes, sorted", got)
	}
}

func TestOPFSReadParsesJSONFilesInTheirDirectory(t *testing.T) {
	records, err := opfsReaderOf(t, notion).Read(writeOPFS(t, notionDir))
	testcheck.NoError(t, err)
	chat := recordOf(t, records, "c1.json")
	if chat.Origin != notion || chat.Kind != webstore.KindOPFS || chat.Namespace != "cache/chats" || chat.Value.Get("messages").Items[0].Get("text").String() != "oi" {
		t.Fatalf("chat record = %+v; want the parsed file of Notion in cache/chats", chat)
	}
	if sqlite := recordOf(t, records, "db.sqlite3"); sqlite.DecodeErr == nil {
		t.Fatalf("sqlite record = %+v; want a DecodeErr, it is not JSON", sqlite)
	}
}

func TestOPFSReadRefusesAnOriginOutsideTheScope(t *testing.T) {
	location := writeOPFS(t, "https+++app.notion.com^partitionKey=%28https%2Cexample.com%29")
	if records, err := opfsReaderOf(t, notion).Read(location); err == nil || records != nil || !strings.Contains(err.Error(), "partitionKey") {
		t.Fatalf("Read = %+v, %v; want a refusal naming the partitioned origin", records, err)
	}
}

func TestOPFSReadRefusesAnEmptyScope(t *testing.T) {
	if _, err := opfsReaderOf(t).Read(writeOPFS(t, notionDir)); !errors.Is(err, sitestorage.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestOPFSRecognizesOnlyAnFsDirectory(t *testing.T) {
	location := writeOPFS(t, notionDir)
	reader := opfsReaderOf(t, notion)
	if !reader.Recognizes(location) || reader.Recognizes(filepath.Dir(location)) {
		t.Fatalf("Recognizes(%q) wrong; want only the fs directory", location)
	}
}

func TestOriginOfDirUndoesFirefoxNaming(t *testing.T) {
	for dir, origin := range map[string]string{notionDir: notion, "http+++localhost+8080": "http://localhost:8080", "moz-extension": "moz-extension"} {
		if got := originOfDir(dir); got != origin {
			t.Errorf("originOfDir(%q) = %q; want %q", dir, got, origin)
		}
	}
}
