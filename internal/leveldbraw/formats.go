package leveldbraw

import (
	"encoding/binary"
	"fmt"
)

// Internal keys end with 8 bytes: sequence << 8 | type.
const internalKeyTrailerLen = 8

// Value types stored in the key trailer and in write batches.
const (
	typeDeletion = 0
	typeValue    = 1
)

// A write batch starts with the sequence of its first record and a count.
const batchHeaderLen = 8 + 4

func parseInternalKey(internalKey []byte) ([]byte, uint64, bool, error) {
	if len(internalKey) < internalKeyTrailerLen {
		return nil, 0, false, fmt.Errorf("internal key %x is %d bytes, expected at least %d", internalKey, len(internalKey), internalKeyTrailerLen)
	}
	split := len(internalKey) - internalKeyTrailerLen
	trailer := binary.LittleEndian.Uint64(internalKey[split:])
	return internalKey[:split], trailer >> 8, trailer&0xff == typeDeletion, nil
}

// decodeBatch applies every record of a journal write batch to versions.
func decodeBatch(batch []byte, versions *versionSet) error {
	if len(batch) < batchHeaderLen {
		return fmt.Errorf("write batch of %d bytes, expected at least %d-byte header", len(batch), batchHeaderLen)
	}
	sequence := binary.LittleEndian.Uint64(batch)
	count := binary.LittleEndian.Uint32(batch[8:])
	rest := batch[batchHeaderLen:]
	for index := uint64(0); index < uint64(count); index++ {
		var err error
		if rest, err = decodeBatchRecord(rest, sequence+index, versions); err != nil {
			return fmt.Errorf("batch at sequence %d, record %d of %d: %w", sequence, index, count, err)
		}
	}
	return nil
}

func decodeBatchRecord(data []byte, sequence uint64, versions *versionSet) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("record truncated before its type byte")
	}
	recordType, rest := data[0], data[1:]
	key, rest, err := readLengthPrefixed(rest)
	if err != nil || recordType == typeDeletion {
		versions.record(key, sequence, true, nil)
		return rest, err
	}
	if recordType != typeValue {
		return nil, fmt.Errorf("record type %d, expected %d (deletion) or %d (value)", recordType, typeDeletion, typeValue)
	}
	value, rest, err := readLengthPrefixed(rest)
	versions.record(key, sequence, false, value)
	return rest, err
}

func readLengthPrefixed(data []byte) ([]byte, []byte, error) {
	length, width := binary.Uvarint(data)
	if width <= 0 || uint64(len(data)-width) < length {
		return nil, nil, fmt.Errorf("length-prefixed slice with length %d does not fit in %d bytes", length, len(data))
	}
	end := width + int(length)
	return data[width:end], data[end:], nil
}
