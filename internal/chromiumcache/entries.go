// Package chromiumcache reads the request caches of Chromium and of
// Electron apps (Discord's desktop app among them): the HTTP cache
// (Cache/Cache_Data) and the Cache API (Service Worker/CacheStorage), both
// kept by the simple cache backend.
package chromiumcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/webstore"
)

// indexDir sits in every simple cache directory, beside its entry files.
const indexDir = "index-dir"

// readEntry turns one opened entry into a record, or reports it out of
// scope; it reads the entry's streams only for a URL in scope.
type readEntry func(entry simplecache.Entry) (webstore.Record, bool)

// readEntryFiles applies read to every entry file of dir. An entry that
// cannot be opened is skipped: Chromium writes and evicts entries while it
// runs, and an entry with no readable key has no URL to be counted against.
func readEntryFiles(dir string, read readEntry) ([]webstore.Record, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list Chromium cache %q: %w", dir, err)
	}
	var records []webstore.Record
	for _, file := range files {
		if !simplecache.EntryFilePattern.MatchString(file.Name()) {
			continue
		}
		if record, inScope := readEntryFile(filepath.Join(dir, file.Name()), read); inScope {
			records = append(records, record)
		}
	}
	return records, nil
}

func readEntryFile(path string, read readEntry) (webstore.Record, bool) {
	file, err := os.Open(path)
	if err != nil {
		return webstore.Record{}, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return webstore.Record{}, false
	}
	entry, err := simplecache.OpenEntry(file, info.Size())
	if err != nil {
		return webstore.Record{}, false
	}
	return read(entry)
}

// isSimpleCache reports whether dir is a simple cache directory.
func isSimpleCache(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, indexDir))
	return err == nil && info.IsDir()
}
