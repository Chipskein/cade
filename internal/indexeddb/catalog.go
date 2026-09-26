package indexeddb

import (
	"encoding/binary"

	"github.com/chipskein/cade/internal/leveldbraw"
)

// storeRef addresses one object store.
type storeRef struct {
	databaseID    uint64
	objectStoreID uint64
}

// catalog maps numeric ids to the names the web page chose.
type catalog struct {
	databases map[uint64]string
	stores    map[storeRef]string
}

func buildCatalog(entries []leveldbraw.Entry) catalog {
	names := catalog{databases: map[uint64]string{}, stores: map[storeRef]string{}}
	for _, entry := range entries {
		prefix, suffix, err := decodeKeyPrefix(entry.Key)
		if err != nil || prefix.indexID != 0 || len(suffix) == 0 {
			continue
		}
		if prefix.databaseID == 0 {
			names.addDatabase(suffix, entry.Value)
			continue
		}
		names.addStore(prefix.databaseID, suffix, entry.Value)
	}
	return names
}

// addDatabase handles DatabaseNameKey: 201, origin, name -> varint id.
func (c catalog) addDatabase(suffix, value []byte) {
	if suffix[0] != databaseNameTypeByte {
		return
	}
	_, rest, err := decodeStringWithLength(suffix[1:])
	if err != nil {
		return
	}
	name, _, err := decodeStringWithLength(rest)
	id, width := binary.Uvarint(value)
	if err == nil && width > 0 {
		c.databases[id] = name
	}
}

// addStore handles ObjectStoreMetaDataKey: 50, varint store id, 0 (name)
// -> UTF-16BE name.
func (c catalog) addStore(databaseID uint64, suffix, value []byte) {
	if suffix[0] != objectStoreMetaTypeByte {
		return
	}
	storeID, width := binary.Uvarint(suffix[1:])
	if width <= 0 || len(suffix) != 1+width+1 || suffix[1+width] != objectStoreNameMetaType {
		return
	}
	c.stores[storeRef{databaseID: databaseID, objectStoreID: storeID}] = decodeUTF16BE(value)
}
