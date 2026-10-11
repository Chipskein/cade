// Package chromiumstorage reads the site storages of Chromium, Chrome and
// Electron apps: localStorage, one LevelDB for every origin of a profile,
// and the Origin Private File System, a directory per origin.
package chromiumstorage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/leveldbraw"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/webstore"
)

// A profile keeps localStorage in "Local Storage/leveldb"; a stored item's
// key is "_" + origin + NUL + the encoded item key, beside VERSION, META:
// and METAACCESS: keys (components/services/storage/dom_storage).
const (
	localStorageDir    = "Local Storage"
	localStorageLevel  = "leveldb"
	leveldbCurrentFile = "CURRENT"
	itemKeyPrefix      = '_'
	originSeparator    = 0
)

// LocalStorageReader reads the items of the origins in its scope.
type LocalStorageReader struct {
	scope sitestorage.Scope
}

// NewLocalStorageReader builds a reader of scope, the configured origins.
//
//	reader := chromiumstorage.NewLocalStorageReader(scope)
//	records, err := reader.Read("~/.config/google-chrome/Default/Local Storage/leveldb")
func NewLocalStorageReader(scope sitestorage.Scope) LocalStorageReader {
	return LocalStorageReader{scope: scope}
}

func (LocalStorageReader) Kind() webstore.Kind { return webstore.KindLocalStorage }

// Recognizes a localStorage LevelDB by its path: an IndexedDB directory is
// a LevelDB too.
func (LocalStorageReader) Recognizes(location string) bool {
	clean := filepath.Clean(location)
	if filepath.Base(clean) != localStorageLevel || filepath.Base(filepath.Dir(clean)) != localStorageDir {
		return false
	}
	info, err := os.Stat(filepath.Join(clean, leveldbCurrentFile))
	return err == nil && info.Mode().IsRegular()
}

// Read returns a record per item of an origin in scope, its container the
// item's key; the items of every other origin are not decoded.
func (r LocalStorageReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, sitestorage.ErrEmptyScope
	}
	entries, err := leveldbraw.ReadLatest(location)
	if err != nil {
		return nil, fmt.Errorf("read Chromium localStorage: %w", err)
	}
	var records []webstore.Record
	for _, entry := range entries {
		origin, key, isItem := splitItemKey(entry.Key)
		if isItem && r.scope.Allows(origin) {
			records = append(records, itemRecord(origin, key, entry.Value))
		}
	}
	return records, nil
}

// splitItemKey parses "_" + origin + NUL + encoded key.
func splitItemKey(stored []byte) (string, []byte, bool) {
	if len(stored) == 0 || stored[0] != itemKeyPrefix {
		return "", nil, false
	}
	origin, key, found := bytes.Cut(stored[1:], []byte{originSeparator})
	return string(origin), key, found
}

func itemRecord(origin string, storedKey, storedValue []byte) webstore.Record {
	record := webstore.Record{Kind: webstore.KindLocalStorage, Origin: origin}
	key, err := decodeStorageString(storedKey)
	if err != nil {
		record.DecodeErr = fmt.Errorf("localStorage key of %s: %w", origin, err)
		return record
	}
	record.Container, record.Key = key, key
	text, err := decodeStorageString(storedValue)
	if err != nil {
		record.DecodeErr = fmt.Errorf("localStorage item %q of %s: %w", key, origin, err)
		return record
	}
	record.Value = sitestorage.TextValue(text)
	return record
}
