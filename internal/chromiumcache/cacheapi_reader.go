package chromiumcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/protoscan"
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/webstore"
)

// An origin's CacheStorage directory holds index.txt, a CacheStorageIndex
// message (content/browser/cache_storage/cache_storage.proto), and one
// simple cache directory per cache, keyed by the request URL.
const (
	cacheIndexFile = "index.txt"
	// CacheStorageIndex.cache, and in it Cache.name and Cache.cache_dir.
	indexCacheField    = 1
	cacheNameField     = 1
	cacheDirNameField  = 2
	cacheAPIDirsPrefix = "Service Worker/CacheStorage/<origin hash>"
)

// CacheAPIReader reads the Cache API caches of one origin whose request
// URLs are in its scope.
type CacheAPIReader struct {
	scope requestcache.Scope
}

// NewCacheAPIReader builds a reader of scope, the configured URLs.
//
//	reader := chromiumcache.NewCacheAPIReader(scope)
//	records, err := reader.Read("~/.config/discord/Service Worker/CacheStorage/63fc7e4c…")
func NewCacheAPIReader(scope requestcache.Scope) CacheAPIReader {
	return CacheAPIReader{scope: scope}
}

func (CacheAPIReader) Kind() webstore.Kind { return webstore.KindCacheAPI }

// Recognizes an origin's CacheStorage directory by its index.
func (CacheAPIReader) Recognizes(location string) bool {
	info, err := os.Stat(filepath.Join(location, cacheIndexFile))
	return err == nil && info.Mode().IsRegular()
}

// Read returns a record per cached response in scope, its namespace the
// cache it was put in.
func (r CacheAPIReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, requestcache.ErrEmptyScope
	}
	caches, err := readCacheIndex(location)
	if err != nil {
		return nil, err
	}
	var records []webstore.Record
	for _, cache := range caches {
		found, err := readEntryFiles(filepath.Join(location, cache.dir), r.entryReader(cache.name))
		if err != nil {
			return nil, err
		}
		records = append(records, found...)
	}
	return records, nil
}

// entryReader reads the entries of the cache named cacheName. The Cache
// API keeps a body as the page received it, already decoded.
func (r CacheAPIReader) entryReader(cacheName string) readEntry {
	return func(entry simplecache.Entry) (webstore.Record, bool) {
		name, inScope := r.scope.Match(entry.Key)
		if !inScope {
			return webstore.Record{}, false
		}
		_, body, err := entry.Streams()
		if err != nil {
			return webstore.Record{}, false
		}
		response := requestcache.Response{URL: entry.Key, Namespace: cacheName, Body: body}
		return response.Record(webstore.KindCacheAPI, name), true
	}
}

// namedCache is one cache of the index: its name and its directory.
type namedCache struct {
	name string
	dir  string
}

func readCacheIndex(location string) ([]namedCache, error) {
	raw, err := os.ReadFile(filepath.Join(location, cacheIndexFile))
	if err != nil {
		return nil, fmt.Errorf("read Cache API index of %q: %w", location, err)
	}
	fields, err := protoscan.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("index.txt of the Cache API in %q: %w", location, err)
	}
	var caches []namedCache
	for _, rawCache := range protoscan.Bytes(fields, indexCacheField) {
		cache, err := parseCache(rawCache)
		if err != nil {
			return nil, fmt.Errorf("index.txt of the Cache API in %q: %w", location, err)
		}
		caches = append(caches, cache)
	}
	return caches, nil
}

func parseCache(raw []byte) (namedCache, error) {
	fields, err := protoscan.Parse(raw)
	if err != nil {
		return namedCache{}, err
	}
	cache := namedCache{name: protoscan.String(fields, cacheNameField), dir: protoscan.String(fields, cacheDirNameField)}
	// The directory comes from a file on disk; it must not leave the
	// origin's directory.
	if cache.dir == "" || cache.dir != filepath.Base(cache.dir) || cache.dir == ".." {
		return namedCache{}, fmt.Errorf("cache %q in directory %q, expected a directory name inside %s", cache.name, cache.dir, cacheAPIDirsPrefix)
	}
	return cache, nil
}
