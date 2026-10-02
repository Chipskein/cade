package firefoxidb

import (
	"database/sql"
	"fmt"
)

// catalog names one database file: its IndexedDB name and its stores.
type catalog struct {
	database string
	stores   map[int64]string
}

// readCatalog reads the database row (Firefox keeps exactly one per file)
// and the object-store names.
func readCatalog(db *sql.DB) (catalog, error) {
	found := catalog{stores: map[int64]string{}}
	if err := db.QueryRow(`SELECT name FROM database`).Scan(&found.database); err != nil {
		return catalog{}, fmt.Errorf("read database name: %w", err)
	}
	rows, err := db.Query(`SELECT id, name FROM object_store`)
	if err != nil {
		return catalog{}, fmt.Errorf("list object stores: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return catalog{}, fmt.Errorf("read object store row: %w", err)
		}
		found.stores[id] = name
	}
	return found, rows.Err()
}

// storeName falls back to the numeric id, like the Chromium reader, when a
// value points at a store with no metadata row.
func (c catalog) storeName(id int64) string {
	if name, ok := c.stores[id]; ok {
		return name
	}
	return fmt.Sprintf("#%d", id)
}
