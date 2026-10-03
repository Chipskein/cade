package main

import (
	"github.com/chipskein/cade/internal/chromiumcache"
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/firefoxcache"
	"github.com/chipskein/cade/internal/firefoxidb"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/smclone"
	"github.com/chipskein/cade/internal/webstore"
)

// storeReaders are the browser storages this build reads; a new storage is
// one more reader here. The request cache readers come first: a Firefox
// cache directory also holds a .sqlite file, which the IndexedDB reader
// would claim.
//
//	readers, err := storeReaders(cfg.Sources)
func storeReaders(sources config.SourcesConfig) (webstore.Readers, error) {
	scope, err := requestcache.NewScope(sources.RequestCacheURLs)
	if err != nil {
		return nil, err
	}
	return webstore.Readers{
		chromiumcache.NewCacheAPIReader(scope), firefoxcache.NewCacheAPIReader(scope, openSQLiteFile),
		firefoxcache.NewHTTPCacheReader(scope), chromiumcache.NewHTTPCacheReader(scope),
		indexeddb.ChromiumReader{}, firefoxidb.NewReader(smclone.Decode, openSQLiteFile),
	}, nil
}

// readIndexedDB reads a Chromium (*.indexeddb.leveldb) or a Firefox and
// Floorp (idb, of *.sqlite files) IndexedDB directory, told apart by
// layout.
//
//	records, err := readIndexedDB("~/.floorp/x.default/storage/default/https+++teams.microsoft.com/idb")
func readIndexedDB(dir string) ([]indexeddb.Record, error) {
	readers, err := storeReaders(config.SourcesConfig{})
	if err != nil {
		return nil, err
	}
	return readers.ReadKind(webstore.KindIndexedDB, dir)
}
