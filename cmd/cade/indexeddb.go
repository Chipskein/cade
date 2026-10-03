package main

import (
	"github.com/chipskein/cade/internal/firefoxidb"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/smclone"
	"github.com/chipskein/cade/internal/webstore"
)

// storeReaders are the browser storages this build reads; a new storage is
// one more reader here.
func storeReaders() webstore.Readers {
	return webstore.Readers{indexeddb.ChromiumReader{}, firefoxidb.NewReader(smclone.Decode, openSQLiteFile)}
}

// readIndexedDB reads a Chromium (*.indexeddb.leveldb) or a Firefox and
// Floorp (idb, of *.sqlite files) IndexedDB directory, told apart by
// layout.
//
//	records, err := readIndexedDB("~/.floorp/x.default/storage/default/https+++teams.microsoft.com/idb")
func readIndexedDB(dir string) ([]indexeddb.Record, error) {
	return storeReaders().ReadKind(webstore.KindIndexedDB, dir)
}
