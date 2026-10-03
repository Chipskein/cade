// Package firefoxidb reads Firefox IndexedDB databases (the idb/*.sqlite
// files of an origin under a profile's storage/default) into the same
// records the Chromium reader produces, so schema tools never care which
// browser the data came from.
package firefoxidb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// DecodeClone turns one uncompressed SpiderMonkey structured clone into a
// value tree; injected so the reader is testable without a real decoder.
type DecodeClone func(payload []byte) (*v8value.Value, error)

// OpenDatabase opens a SQLite file read-only; injected so the package does
// not register a driver itself.
type OpenDatabase func(path string) (*sql.DB, error)

// Reader reads every database of one origin's idb directory.
type Reader struct {
	decode DecodeClone
	open   OpenDatabase
}

// NewReader builds a Reader.
//
//	reader := firefoxidb.NewReader(smclone.Decode, openReadOnly)
func NewReader(decode DecodeClone, open OpenDatabase) Reader {
	return Reader{decode: decode, open: open}
}

// sqliteSuffix marks the database files inside an idb directory; each one
// holds a single IndexedDB database.
const sqliteSuffix = ".sqlite"

// ReadDirectory snapshots dir (Firefox keeps the files open and writing)
// and returns every object-store record of every database in it.
//
//	records, err := reader.ReadDirectory("~/.floorp/x.default/storage/default/https+++web.whatsapp.com/idb")
func (r Reader) ReadDirectory(dir string) ([]indexeddb.Record, error) {
	snapshot, err := os.MkdirTemp("", "cade-firefoxidb-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	if err := filecopy.FlatDirectory(dir, snapshot); err != nil {
		return nil, fmt.Errorf("snapshot Firefox IndexedDB %q: %w", dir, err)
	}
	paths, err := filepath.Glob(filepath.Join(snapshot, "*"+sqliteSuffix))
	if err != nil {
		return nil, fmt.Errorf("list databases in %q: %w", snapshot, err)
	}
	records, err := r.readDatabases(paths)
	return webstore.WithOrigin(records, indexeddb.OriginName(dir)), err
}

func (r Reader) readDatabases(paths []string) ([]indexeddb.Record, error) {
	var records []indexeddb.Record
	for _, path := range paths {
		found, err := r.readDatabase(path)
		if err != nil {
			return nil, err
		}
		records = append(records, found...)
	}
	return records, nil
}

func (r Reader) readDatabase(path string) ([]indexeddb.Record, error) {
	db, err := r.open(path)
	if err != nil {
		return nil, fmt.Errorf("open Firefox IndexedDB %q: %w", filepath.Base(path), err)
	}
	defer db.Close()
	catalog, err := readCatalog(db)
	if err != nil {
		return nil, fmt.Errorf("read catalog of %q: %w", filepath.Base(path), err)
	}
	return r.readValues(db, catalog)
}
