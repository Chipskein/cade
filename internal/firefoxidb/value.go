package firefoxidb

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/snappyblock"
)

// externalCloneMarker prefixes a file_ids entry whose file holds the whole
// structured clone (dom/indexedDB/ActorsParent.cpp: values too large for
// the row). Entries without it are Blob/File objects referenced from an
// inline clone, which still decodes.
const externalCloneMarker = "."

func (r Reader) readValues(db *sql.DB, names catalog) ([]indexeddb.Record, error) {
	rows, err := db.Query(`SELECT object_store_id, data, file_ids FROM object_data ORDER BY object_store_id, key`)
	if err != nil {
		return nil, fmt.Errorf("list values of %q: %w", names.database, err)
	}
	defer rows.Close()
	var records []indexeddb.Record
	for rows.Next() {
		var storeID int64
		var stored []byte
		var fileIDs sql.NullString
		if err := rows.Scan(&storeID, &stored, &fileIDs); err != nil {
			return nil, fmt.Errorf("read value row of %q: %w", names.database, err)
		}
		records = append(records, r.decodeRecord(names, storeID, stored, fileIDs.String))
	}
	return records, rows.Err()
}

func (r Reader) decodeRecord(names catalog, storeID int64, stored []byte, fileIDs string) indexeddb.Record {
	record := indexeddb.Record{Database: names.database, Store: names.storeName(storeID)}
	payload, err := clonePayload(stored, fileIDs)
	if err != nil {
		record.DecodeErr = err
		return record
	}
	record.Value, record.DecodeErr = r.decode(payload)
	return record
}

// clonePayload undoes the Snappy block compression Firefox applies to every
// inline value.
func clonePayload(stored []byte, fileIDs string) ([]byte, error) {
	if hasExternalClone(fileIDs) {
		return nil, indexeddb.ErrBlobWrapped
	}
	return snappyblock.Decode(stored)
}

// hasExternalClone reports whether fileIDs (space-separated ids, e.g.
// ".12 7") names a file holding the clone itself.
func hasExternalClone(fileIDs string) bool {
	for _, id := range strings.Fields(fileIDs) {
		if strings.HasPrefix(id, externalCloneMarker) {
			return true
		}
	}
	return false
}
