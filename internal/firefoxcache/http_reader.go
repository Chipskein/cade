package firefoxcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/webstore"
)

// entriesDir holds one file per cached response inside cache2, named
// after the SHA-1 of its key.
const entriesDir = "entries"

// HTTPCacheReader reads the responses of a cache2 directory whose URLs are
// in its scope.
type HTTPCacheReader struct {
	scope requestcache.Scope
}

// NewHTTPCacheReader builds a reader of scope, the configured URLs.
//
//	reader := firefoxcache.NewHTTPCacheReader(scope)
//	records, err := reader.Read("~/.cache/floorp/x.default/cache2")
func NewHTTPCacheReader(scope requestcache.Scope) HTTPCacheReader {
	return HTTPCacheReader{scope: scope}
}

func (HTTPCacheReader) Kind() webstore.Kind { return webstore.KindHTTPCache }

// Recognizes a cache2 directory by its entries directory.
func (HTTPCacheReader) Recognizes(location string) bool {
	info, err := os.Stat(filepath.Join(location, entriesDir))
	return err == nil && info.IsDir()
}

// Read returns a record per entry in scope. An entry that cannot be read
// is skipped: Firefox writes and dooms entries while it runs, and an entry
// whose metadata is unreadable has no URL to be counted against.
func (r HTTPCacheReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, requestcache.ErrEmptyScope
	}
	dir := filepath.Join(location, entriesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list Firefox HTTP cache %q: %w", dir, err)
	}
	var records []webstore.Record
	for _, entry := range entries {
		if record, inScope := r.readFile(filepath.Join(dir, entry.Name())); inScope {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r HTTPCacheReader) readFile(path string) (webstore.Record, bool) {
	file, err := os.Open(path)
	if err != nil {
		return webstore.Record{}, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return webstore.Record{}, false
	}
	record, inScope, err := readEntry(file, info.Size(), r.scope)
	return record, inScope && err == nil
}
