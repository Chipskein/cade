package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadIndexedDBReadsFirefoxDirectories(t *testing.T) {
	records, err := readIndexedDB("../../testdata/firefox-indexeddb")
	if err != nil || len(records) != 3 || records[0].Database != "model-storage-fixture" {
		t.Fatalf("expected the 3 Floorp sample records, got %d (err %v)", len(records), err)
	}
}

func TestReadIndexedDBReadsChromiumDirectories(t *testing.T) {
	records, err := readIndexedDB("../../testdata/chrome-indexeddb.leveldb")
	if err != nil || len(records) == 0 || records[0].Database != "Teams:replychain-manager:fixture" {
		t.Fatalf("expected the Chrome sample records, got %d (err %v)", len(records), err)
	}
}

func TestIsFirefoxIndexedDBFalseForMissingDirectory(t *testing.T) {
	if isFirefoxIndexedDB(filepath.Join(t.TempDir(), "absent")) {
		t.Fatal("a missing directory is not a Firefox idb directory")
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
