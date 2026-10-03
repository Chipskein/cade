package main

import (
	"os"
	"strings"

	"github.com/chipskein/cade/internal/firefoxidb"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/smclone"
)

// firefoxDatabaseSuffix marks the files of a Firefox idb directory: one
// SQLite database per IndexedDB database.
const firefoxDatabaseSuffix = ".sqlite"

// readIndexedDB picks the reader by directory layout: Chromium keeps an
// origin's IndexedDB in a *.indexeddb.leveldb directory, Firefox and
// Floorp in an idb directory of *.sqlite files.
//
//	records, err := readIndexedDB("~/.floorp/x.default/storage/default/https+++teams.microsoft.com/idb")
func readIndexedDB(dir string) ([]indexeddb.Record, error) {
	if isFirefoxIndexedDB(dir) {
		return firefoxidb.NewReader(smclone.Decode, openSQLiteFile).ReadDirectory(dir)
	}
	return indexeddb.ReadDirectory(dir)
}

func isFirefoxIndexedDB(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), firefoxDatabaseSuffix) {
			return true
		}
	}
	return false
}
