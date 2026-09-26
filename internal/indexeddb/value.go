package indexeddb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/snappyblock"
)

// Blink value envelopes (third_party/blink/.../idb_value_wrapping.h): a
// pseudo SSV version 17 followed by a command marks processed values.
var (
	snappyWrappedPrefix = []byte{0xFF, 0x11, 0x02}
	blobWrappedPrefix   = []byte{0xFF, 0x11, 0x01}
)

// Blink's own serialization header: 0xFF, version varint, and since version
// 21 a trailer-offset tag followed by uint64 offset and uint32 size.
const (
	blinkVersionTag       = 0xFF
	blinkTrailerTag       = 0xFE
	blinkTrailerFieldsLen = 8 + 4
)

// ErrBlobWrapped reports a value too large to live in LevelDB; Chromium
// moved it to a file in the sibling .blob directory.
var ErrBlobWrapped = errors.New("value stored in an external blob file")

// v8Payload turns a stored object-store value into the bytes v8value.Decode
// expects: it drops the record version, undoes Snappy wrapping and strips
// the Blink header.
func v8Payload(stored []byte) ([]byte, error) {
	_, width := binary.Uvarint(stored)
	if width <= 0 {
		return nil, fmt.Errorf("record of %d bytes lacks its version varint", len(stored))
	}
	serialized := stored[width:]
	if bytes.HasPrefix(serialized, blobWrappedPrefix) {
		return nil, ErrBlobWrapped
	}
	if bytes.HasPrefix(serialized, snappyWrappedPrefix) {
		unwrapped, err := snappyblock.Decode(serialized[len(snappyWrappedPrefix):])
		if err != nil {
			return nil, err
		}
		serialized = unwrapped
	}
	return stripBlinkHeader(serialized)
}

func stripBlinkHeader(serialized []byte) ([]byte, error) {
	if len(serialized) == 0 || serialized[0] != blinkVersionTag {
		return nil, fmt.Errorf("serialized value starts with %x, expected Blink header byte %#x", serialized[:min(len(serialized), 4)], blinkVersionTag)
	}
	_, width := binary.Uvarint(serialized[1:])
	if width <= 0 {
		return nil, fmt.Errorf("invalid Blink version varint %x", serialized[:min(len(serialized), 6)])
	}
	rest := serialized[1+width:]
	if len(rest) > 0 && rest[0] == blinkTrailerTag {
		if len(rest) < 1+blinkTrailerFieldsLen {
			return nil, fmt.Errorf("Blink trailer offset needs %d bytes, %d remain", 1+blinkTrailerFieldsLen, len(rest))
		}
		rest = rest[1+blinkTrailerFieldsLen:]
	}
	return rest, nil
}
