package main

import (
	"github.com/chipskein/cade/internal/chromiumcache"
	"github.com/chipskein/cade/internal/chromiumstorage"
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/firefoxcache"
	"github.com/chipskein/cade/internal/firefoxidb"
	"github.com/chipskein/cade/internal/firefoxstorage"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/smclone"
	"github.com/chipskein/cade/internal/webstore"
)

// storeReaders are the browser storages this build reads; a new storage is
// one more reader here. The request cache and site storage readers come
// first: a Firefox cache, ls or fs directory also holds a .sqlite file,
// and Chromium's localStorage is a LevelDB, which the IndexedDB readers
// would claim.
//
//	readers, err := storeReaders(cfg.Sources)
func storeReaders(sources config.SourcesConfig) (webstore.Readers, error) {
	urls, err := requestcache.NewScope(sources.RequestCacheURLs)
	if err != nil {
		return nil, err
	}
	origins, err := sitestorage.NewScope(sources.StorageOrigins)
	if err != nil {
		return nil, err
	}
	return webstore.Readers{
		chromiumcache.NewCacheAPIReader(urls), firefoxcache.NewCacheAPIReader(urls, openSQLiteFile),
		firefoxcache.NewHTTPCacheReader(urls), chromiumcache.NewHTTPCacheReader(urls),
		chromiumstorage.NewLocalStorageReader(origins), firefoxstorage.NewLocalStorageReader(origins, openSQLiteFile),
		chromiumstorage.NewOPFSReader(origins), firefoxstorage.NewOPFSReader(origins, openSQLiteFile),
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
