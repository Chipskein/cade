// Package indexeddb reads Chromium IndexedDB databases (the
// *.indexeddb.leveldb directories of a browser profile) into decoded records.
package indexeddb

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/leveldbraw"
	"github.com/chipskein/cade/internal/v8value"
)

// Record is one object-store value. Exactly one of Value and DecodeErr is
// set: a record that fails to decode is still reported, so callers can
// count what they could not read instead of silently losing it.
type Record struct {
	Database  string
	Store     string
	Value     *v8value.Value
	DecodeErr error
}

// ReadDirectory snapshots dir (the browser keeps it locked and writing)
// and returns every live object-store record.
//
//	records, err := indexeddb.ReadDirectory("~/.config/google-chrome/Default/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb")
func ReadDirectory(dir string) ([]Record, error) {
	snapshot, err := os.MkdirTemp("", "cade-indexeddb-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	copyDir := filepath.Join(snapshot, "db")
	if err := filecopy.FlatDirectory(dir, copyDir); err != nil {
		return nil, fmt.Errorf("snapshot IndexedDB %q: %w", dir, err)
	}
	entries, err := leveldbraw.ReadLatest(copyDir)
	if err != nil {
		return nil, err
	}
	return decodeRecords(entries), nil
}

func decodeRecords(entries []leveldbraw.Entry) []Record {
	names := buildCatalog(entries)
	var records []Record
	for _, entry := range entries {
		prefix, _, err := decodeKeyPrefix(entry.Key)
		if err != nil || prefix.databaseID == 0 || prefix.indexID != objectStoreDataIndexID {
			continue
		}
		records = append(records, decodeRecord(names, prefix, entry.Value))
	}
	return records
}

func decodeRecord(names catalog, prefix keyPrefix, stored []byte) Record {
	record := Record{
		Database: nameOrID(names.databases[prefix.databaseID], prefix.databaseID),
		Store:    nameOrID(names.stores[storeRef{prefix.databaseID, prefix.objectStoreID}], prefix.objectStoreID),
	}
	payload, err := v8Payload(stored)
	if err != nil {
		record.DecodeErr = err
		return record
	}
	record.Value, record.DecodeErr = v8value.Decode(payload)
	return record
}

// nameOrID falls back to the numeric id when the metadata row is missing
// (e.g. it lives in a table that was compacted away mid-copy).
func nameOrID(name string, id uint64) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("#%d", id)
}
