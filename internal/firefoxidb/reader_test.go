package firefoxidb

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/snappy"
	_ "github.com/mattn/go-sqlite3"
)

// FakeCloneDecoder stands in for the SpiderMonkey decoder: it returns the
// payload as a string value, so tests can see exactly what reached it.
type FakeCloneDecoder struct {
	FailWith error
}

func (f FakeCloneDecoder) Decode(payload []byte) (*v8value.Value, error) {
	if f.FailWith != nil {
		return nil, f.FailWith
	}
	return &v8value.Value{Kind: v8value.KindString, Text: string(payload)}, nil
}

func openForTest(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

// The tables Firefox creates in every idb/*.sqlite file, trimmed to the
// columns the reader uses (dom/indexedDB/DBSchema.cpp).
const firefoxTables = `
CREATE TABLE database (name TEXT PRIMARY KEY, origin TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 0);
CREATE TABLE object_store (id INTEGER PRIMARY KEY, auto_increment INTEGER NOT NULL DEFAULT 0, name TEXT NOT NULL, key_path TEXT);
CREATE TABLE object_data (object_store_id INTEGER NOT NULL, key BLOB NOT NULL, index_data_values BLOB DEFAULT NULL, file_ids TEXT, data BLOB NOT NULL, PRIMARY KEY (object_store_id, key)) WITHOUT ROWID;`

// fixtureValue is one object_data row before compression.
type fixtureValue struct {
	storeID int64
	key     string
	plain   string
	fileIDs any
}

func writeFixtureDatabase(t *testing.T, path, name string, stores map[int64]string, values []fixtureValue) {
	t.Helper()
	db, err := openForTest(path)
	if err != nil {
		t.Fatalf("open fixture %q: %v", path, err)
	}
	defer db.Close()
	mustExec(t, db, firefoxTables)
	mustExec(t, db, `INSERT INTO database (name, origin) VALUES (?, 'https://example.test')`, name)
	for id, store := range stores {
		mustExec(t, db, `INSERT INTO object_store (id, name) VALUES (?, ?)`, id, store)
	}
	for _, value := range values {
		compressed := snappy.Encode(nil, []byte(value.plain))
		mustExec(t, db, `INSERT INTO object_data (object_store_id, key, data, file_ids) VALUES (?, ?, ?, ?)`, value.storeID, value.key, compressed, value.fileIDs)
	}
}

func mustExec(t *testing.T, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

func readFixture(t *testing.T, dir string, decoder FakeCloneDecoder) []indexeddb.Record {
	t.Helper()
	records, err := NewReader(decoder.Decode, openForTest).ReadDirectory(dir)
	if err != nil {
		t.Fatalf("read %q: %v", dir, err)
	}
	return records
}

func TestReadDirectoryNamesDatabasesAndStores(t *testing.T) {
	dir := t.TempDir()
	writeFixtureDatabase(t, filepath.Join(dir, "1abc.sqlite"), "model-storage", map[int64]string{1: "chat", 2: "message"},
		[]fixtureValue{{1, "a", "chat-a", nil}, {2, "b", "msg-b", nil}, {2, "c", "msg-c", nil}})
	writeFixtureDatabase(t, filepath.Join(dir, "2def.sqlite"), "wawc", map[int64]string{1: "user"},
		[]fixtureValue{{1, "u", "user-u", nil}})
	got := map[string]string{}
	for _, record := range readFixture(t, dir, FakeCloneDecoder{}) {
		got[record.Namespace+"/"+record.Container+"/"+record.Value.Text] = ""
	}
	for _, want := range []string{"model-storage/chat/chat-a", "model-storage/message/msg-b", "model-storage/message/msg-c", "wawc/user/user-u"} {
		if _, ok := got[want]; !ok || len(got) != 4 {
			t.Fatalf("expected %q among 4 decompressed records, got %v", want, got)
		}
	}
}

func TestReaderRecognizesIdbDirectoriesOnly(t *testing.T) {
	reader := NewReader(FakeCloneDecoder{}.Decode, openForTest)
	if !reader.Recognizes(sampleDir) || reader.Kind() != webstore.KindIndexedDB {
		t.Fatalf("Recognizes(%q) = false or kind %q; want an indexeddb idb directory", sampleDir, reader.Kind())
	}
	for _, other := range []string{"../../testdata/chrome-indexeddb.leveldb", filepath.Join(t.TempDir(), "absent")} {
		if reader.Recognizes(other) {
			t.Fatalf("Recognizes(%q) = true; want false without a .sqlite file", other)
		}
	}
}

func TestReadDirectoryReportsExternalClonesAsBlobWrapped(t *testing.T) {
	dir := t.TempDir()
	writeFixtureDatabase(t, filepath.Join(dir, "db.sqlite"), "db", map[int64]string{1: "s"},
		[]fixtureValue{{1, "big", "ignored", ".3"}, {1, "withBlob", "inline", "4"}})
	records := readFixture(t, dir, FakeCloneDecoder{})
	if len(records) != 2 || !errors.Is(records[0].DecodeErr, indexeddb.ErrBlobWrapped) {
		t.Fatalf("expected the .3 value to be blob wrapped, got %+v", records)
	}
	if records[1].DecodeErr != nil || records[1].Value.Text != "inline" {
		t.Fatalf("expected the value that only references a blob to decode, got %+v", records[1])
	}
}

func TestReadDirectoryKeepsRecordsThatFailToDecode(t *testing.T) {
	dir := t.TempDir()
	writeFixtureDatabase(t, filepath.Join(dir, "db.sqlite"), "db", map[int64]string{1: "s"}, []fixtureValue{{1, "k", "x", nil}})
	failure := errors.New("unknown tag")
	records := readFixture(t, dir, FakeCloneDecoder{FailWith: failure})
	if len(records) != 1 || !errors.Is(records[0].DecodeErr, failure) || records[0].Container != "s" {
		t.Fatalf("expected one failed record in store s, got %+v", records)
	}
}

func TestReadDirectoryNamesUnknownStoresByID(t *testing.T) {
	dir := t.TempDir()
	writeFixtureDatabase(t, filepath.Join(dir, "db.sqlite"), "db", map[int64]string{}, []fixtureValue{{7, "k", "x", nil}})
	if records := readFixture(t, dir, FakeCloneDecoder{}); records[0].Container != "#7" {
		t.Fatalf("expected store #7, got %q", records[0].Container)
	}
}

func TestReadDirectoryRejectsCorruptCompression(t *testing.T) {
	if _, err := clonePayload([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, ""); err == nil {
		t.Fatal("expected an error for a value that is not Snappy")
	}
}

func TestReadDirectoryFailsOnMissingDirectory(t *testing.T) {
	_, err := NewReader(FakeCloneDecoder{}.Decode, openForTest).ReadDirectory(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected an error for a missing idb directory")
	}
}

func TestReadDirectoryFailsOnFileWithoutCatalog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.sqlite"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewReader(FakeCloneDecoder{}.Decode, openForTest).ReadDirectory(dir)
	if err == nil {
		t.Fatal("expected an error for a SQLite file without the database table")
	}
}
