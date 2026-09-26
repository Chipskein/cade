package indexeddb

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// Chromium key layout (content/browser/indexed_db/indexed_db_leveldb_coding.h):
// a prefix byte packing the byte widths of (database id, object store id,
// index id), the three ids little-endian, then a type-specific suffix.
const (
	objectStoreDataIndexID  = 1
	databaseNameTypeByte    = 201
	objectStoreMetaTypeByte = 50
	objectStoreNameMetaType = 0
)

// keyPrefix identifies what a key belongs to.
type keyPrefix struct {
	databaseID    uint64
	objectStoreID uint64
	indexID       uint64
}

func decodeKeyPrefix(key []byte) (keyPrefix, []byte, error) {
	if len(key) == 0 {
		return keyPrefix{}, nil, fmt.Errorf("empty IndexedDB key, expected a prefix byte")
	}
	widths := [3]int{int(key[0]>>5) + 1, int(key[0]>>2&7) + 1, int(key[0]&3) + 1}
	rest := key[1:]
	var ids [3]uint64
	for i, width := range widths {
		if len(rest) < width {
			return keyPrefix{}, nil, fmt.Errorf("key %x: prefix id %d needs %d bytes, %d remain", key, i, width, len(rest))
		}
		ids[i], rest = littleEndianInt(rest[:width]), rest[width:]
	}
	return keyPrefix{databaseID: ids[0], objectStoreID: ids[1], indexID: ids[2]}, rest, nil
}

func littleEndianInt(raw []byte) uint64 {
	var value uint64
	for i := len(raw) - 1; i >= 0; i-- {
		value = value<<8 | uint64(raw[i])
	}
	return value
}

// decodeStringWithLength reads a varint count of UTF-16 code units followed
// by the big-endian units.
func decodeStringWithLength(data []byte) (string, []byte, error) {
	units, width := binary.Uvarint(data)
	if width <= 0 || uint64(len(data)-width) < 2*units {
		return "", nil, fmt.Errorf("string of %d UTF-16 units does not fit in %d bytes", units, len(data))
	}
	end := width + int(2*units)
	return decodeUTF16BE(data[width:end]), data[end:], nil
}

func decodeUTF16BE(raw []byte) string {
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units))
}
