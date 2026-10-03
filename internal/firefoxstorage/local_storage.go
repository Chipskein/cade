// Package firefoxstorage reads the site storages of Firefox and Floorp:
// localStorage, one SQLite file per origin under storage/default.
package firefoxstorage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/webstore"
)

// OpenDatabase opens a SQLite file read-only; injected so the package does
// not register a driver itself.
type OpenDatabase func(path string) (*sql.DB, error)

// An origin's ls directory holds data.sqlite (dom/localstorage,
// ActorsParent.cpp): the origin in the database table, one row per item
// in the data table.
const (
	localStorageDir      = "ls"
	localStorageDatabase = "data.sqlite"
	originQuery          = "SELECT origin FROM database"
	itemsQuery           = "SELECT key, conversion_type, compression_type, value FROM data"
)

// LocalStorageReader reads the items of one origin's localStorage when
// the origin is in its scope.
type LocalStorageReader struct {
	scope sitestorage.Scope
	open  OpenDatabase
}

// NewLocalStorageReader builds a reader of scope, the configured origins.
//
//	reader := firefoxstorage.NewLocalStorageReader(scope, openReadOnly)
//	records, err := reader.Read("~/.floorp/x.default/storage/default/https+++chatgpt.com/ls")
func NewLocalStorageReader(scope sitestorage.Scope, open OpenDatabase) LocalStorageReader {
	return LocalStorageReader{scope: scope, open: open}
}

func (LocalStorageReader) Kind() webstore.Kind { return webstore.KindLocalStorage }

// Recognizes an origin's ls directory by its name and database: an idb
// directory holds SQLite files too.
func (LocalStorageReader) Recognizes(location string) bool {
	if filepath.Base(filepath.Clean(location)) != localStorageDir {
		return false
	}
	info, err := os.Stat(filepath.Join(location, localStorageDatabase))
	return err == nil && info.Mode().IsRegular()
}

// Read snapshots location (Firefox keeps the database open and writing)
// and returns a record per item, its container and key the item's key.
func (r LocalStorageReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, sitestorage.ErrEmptyScope
	}
	snapshot, err := os.MkdirTemp("", "cade-firefoxstorage-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	if err := filecopy.FlatDirectory(location, snapshot); err != nil {
		return nil, fmt.Errorf("snapshot Firefox localStorage %q: %w", location, err)
	}
	db, err := r.open(filepath.Join(snapshot, localStorageDatabase))
	if err != nil {
		return nil, fmt.Errorf("open Firefox localStorage %q: %w", location, err)
	}
	defer db.Close()
	return r.readOrigin(db, location)
}

// readOrigin reads the items only once the origin is known to be in scope.
func (r LocalStorageReader) readOrigin(db *sql.DB, location string) ([]webstore.Record, error) {
	var origin string
	if err := db.QueryRow(originQuery).Scan(&origin); err != nil {
		return nil, fmt.Errorf("read the origin of Firefox localStorage %q: %w", location, err)
	}
	if !r.scope.Allows(origin) {
		return nil, sitestorage.Refusal(origin, location)
	}
	records, err := readItems(db, location)
	return webstore.WithOrigin(records, origin), err
}

func readItems(db *sql.DB, location string) ([]webstore.Record, error) {
	rows, err := db.Query(itemsQuery)
	if err != nil {
		return nil, fmt.Errorf("list localStorage items of %q: %w", location, err)
	}
	defer rows.Close()
	var records []webstore.Record
	for rows.Next() {
		var item storedItem
		if err := rows.Scan(&item.key, &item.conversion, &item.compression, &item.value); err != nil {
			return nil, fmt.Errorf("read localStorage item of %q: %w", location, err)
		}
		records = append(records, item.record())
	}
	return records, rows.Err()
}
