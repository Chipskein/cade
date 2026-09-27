package indexeddb

import (
	"testing"

	"github.com/chipskein/cade/internal/leveldbraw"
)

// IndexedDB keys and values come straight from Chrome's LevelDB; whatever
// they hold, decoding reports undecodable records instead of panicking.
// Run longer with `make fuzz`.

func FuzzDecodeKeyPrefix(f *testing.F) {
	f.Add([]byte{0x00, 3, 7, 1, 0xAA})
	f.Add([]byte{0x3F, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 1})
	f.Fuzz(func(t *testing.T, key []byte) {
		_, _, _ = decodeKeyPrefix(key)
		_, _, _ = decodeStringWithLength(key)
	})
}

func FuzzDecodeRecords(f *testing.F) {
	v8Object := []byte{0xFF, 0x0F, 'o', '"', 2, 'i', 'd', '"', 1, 'x', '{', 1}
	f.Add([]byte{0x00, 1, 1, 1, 0x01}, append([]byte{0x05, 0xFF, 0x10}, v8Object...))
	f.Add([]byte{0x00, 1, 1, 1, 0x01}, append([]byte{0x05}, 0xFF, 0x11, 0x02, 0x00))
	f.Add([]byte{0x00, 1, 1, 1, 0x01}, append([]byte{0x05}, 0xFF, 0x11, 0x01))
	f.Fuzz(func(t *testing.T, key, value []byte) {
		decodeRecords([]leveldbraw.Entry{{Key: key, Value: value}})
	})
}
