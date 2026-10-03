package firefoxcache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/snappyblock"
	"github.com/chipskein/cade/internal/webstore"
)

// OpenDatabase opens a SQLite file read-only; injected so the package does
// not register a driver itself.
type OpenDatabase func(path string) (*sql.DB, error)

// An origin's cache directory (dom/cache/DBSchema.cpp, FileUtils.cpp) holds
// caches.sqlite, which lists every request, and morgue/<n>/<body id>.final,
// one Snappy-framed file per body, where n is the id's last byte.
const (
	cachesDatabase = "caches.sqlite"
	morgueDir      = "morgue"
	bodySuffix     = ".final"
	// contentNamespace holds the caches pages create; the other one is
	// the browser's own.
	contentNamespace = 0
	bodyIDByteDigits = 2
	hexBase          = 16
)

// bodyIDPattern is the shape of a body id: a braced UUID, as Firefox
// writes it, so an id never names a path outside the morgue.
var bodyIDPattern = regexp.MustCompile(`^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`)

// cachedRequests lists the request URL, the cache name and the body of
// every cached response of the content namespace.
const cachedRequests = `
SELECT e.request_url_no_query || e.request_url_query, s.key, e.response_body_id
FROM entries e JOIN storage s ON s.cache_id = e.cache_id
WHERE s.namespace = ? AND e.response_body_id IS NOT NULL
ORDER BY e.id`

// CacheAPIReader reads the Cache API caches of one origin whose request
// URLs are in its scope.
type CacheAPIReader struct {
	scope requestcache.Scope
	open  OpenDatabase
	// readFile reads a body file; tests watch which ones are read.
	readFile func(path string) ([]byte, error)
}

// NewCacheAPIReader builds a reader of scope, the configured URLs.
//
//	reader := firefoxcache.NewCacheAPIReader(scope, openReadOnly)
//	records, err := reader.Read("~/.floorp/x.default/storage/default/https+++discord.com/cache")
func NewCacheAPIReader(scope requestcache.Scope, open OpenDatabase) CacheAPIReader {
	return CacheAPIReader{scope: scope, open: open, readFile: os.ReadFile}
}

func (CacheAPIReader) Kind() webstore.Kind { return webstore.KindCacheAPI }

// Recognizes an origin's cache directory by its database.
func (CacheAPIReader) Recognizes(location string) bool {
	info, err := os.Stat(filepath.Join(location, cachesDatabase))
	return err == nil && info.Mode().IsRegular()
}

// Read snapshots the database (Firefox keeps it open and writing) and
// returns a record per cached response in scope, its namespace the cache
// it was put in. Bodies are read from the live morgue, which Firefox
// never rewrites: a body file is replaced, not edited.
func (r CacheAPIReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, requestcache.ErrEmptyScope
	}
	snapshot, err := os.MkdirTemp("", "cade-firefoxcache-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	if err := filecopy.FlatDirectory(location, snapshot); err != nil {
		return nil, fmt.Errorf("snapshot Firefox Cache API %q: %w", location, err)
	}
	db, err := r.open(filepath.Join(snapshot, cachesDatabase))
	if err != nil {
		return nil, fmt.Errorf("open Firefox Cache API %q: %w", location, err)
	}
	defer db.Close()
	return r.readRequests(db, location)
}

func (r CacheAPIReader) readRequests(db *sql.DB, location string) ([]webstore.Record, error) {
	rows, err := db.Query(cachedRequests, contentNamespace)
	if err != nil {
		return nil, fmt.Errorf("list cached requests of %q: %w", location, err)
	}
	defer rows.Close()
	var records []webstore.Record
	for rows.Next() {
		var url, bodyID string
		var cacheName []byte
		if err := rows.Scan(&url, &cacheName, &bodyID); err != nil {
			return nil, fmt.Errorf("read cached request of %q: %w", location, err)
		}
		if name, inScope := r.scope.Match(url); inScope {
			records = append(records, r.readCachedBody(location, url, decodeCacheName(cacheName), bodyID, name))
		}
	}
	return records, rows.Err()
}

// readCachedBody reads the body of a request in scope; one it cannot read
// is still a record, with DecodeErr, so checks count it.
func (r CacheAPIReader) readCachedBody(location, url, cacheName, bodyID, container string) webstore.Record {
	response := requestcache.Response{URL: url, Namespace: cacheName}
	framed, err := r.readBodyFile(location, bodyID)
	if err == nil {
		response.Body, err = snappyblock.DecodeFramed(framed)
	}
	record := response.Record(webstore.KindCacheAPI, container)
	if err != nil {
		record.Value, record.DecodeErr = nil, fmt.Errorf("body of %s: %w", url, err)
	}
	return record
}

func (r CacheAPIReader) readBodyFile(location, bodyID string) ([]byte, error) {
	if !bodyIDPattern.MatchString(bodyID) {
		return nil, fmt.Errorf("body id %q, expected a braced lowercase UUID", bodyID)
	}
	// The id's last byte, before the closing brace, names its directory.
	lastByte := bodyID[len(bodyID)-1-bodyIDByteDigits : len(bodyID)-1]
	dir, err := strconv.ParseUint(lastByte, hexBase, 8)
	if err != nil {
		return nil, fmt.Errorf("body id %q: %w", bodyID, err)
	}
	return r.readFile(filepath.Join(location, morgueDir, strconv.FormatUint(dir, 10), bodyID+bodySuffix))
}

// decodeCacheName reads a cache name as Firefox stores it: UTF-16LE.
func decodeCacheName(raw []byte) string {
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return string(utf16.Decode(units))
}
