package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadIndexedDBReadsFirefoxDirectories(t *testing.T) {
	records, err := readIndexedDB("../../testdata/firefox-indexeddb")
	if err != nil || len(records) != 3 || records[0].Namespace != "model-storage-fixture" {
		t.Fatalf("expected the 3 Floorp sample records, got %d (err %v)", len(records), err)
	}
}

func TestReadIndexedDBReadsChromiumDirectories(t *testing.T) {
	records, err := readIndexedDB("../../testdata/chrome-indexeddb.leveldb")
	if err != nil || len(records) == 0 || records[0].Namespace != "Teams:replychain-manager:fixture" {
		t.Fatalf("expected the Chrome sample records, got %d (err %v)", len(records), err)
	}
}

func TestReadIndexedDBRefusesADirectoryOfNoKnownLayout(t *testing.T) {
	dir := t.TempDir()
	if _, err := readIndexedDB(dir); err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("readIndexedDB(empty dir) error = %v; want a refusal naming the directory", err)
	}
}

// Firefox names IndexedDB files with "%"; unescaped in a file: URI it would
// be read as an escape and open (or create) another file.
func TestOpenSQLiteFileKeepsURISpecialCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2838935879%e2gF?a#b.sqlite")
	db, err := openSQLiteFile(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE marker (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the database at %q itself, got %v", path, err)
	}
}
