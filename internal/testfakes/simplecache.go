package testfakes

import (
	"bytes"
	"encoding/binary"

	"github.com/chipskein/cade/internal/simplecache"
)

// SimpleCacheEntry is a synthetic Chromium simple cache entry, laid out by
// Bytes as a <hash>_0 file with both streams.
type SimpleCacheEntry struct {
	Key     string
	Stream0 []byte
	Body    []byte
}

// Bytes lays the entry out: header, key, body, its EOF, stream 0, the key
// SHA-256 slot, stream 0's EOF.
func (e SimpleCacheEntry) Bytes() []byte {
	var out bytes.Buffer
	put := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(&out, binary.LittleEndian, value)
		}
	}
	put(uint64(simplecache.InitialMagic), uint32(5), uint32(len(e.Key)), uint32(0), uint32(0))
	out.WriteString(e.Key)
	out.Write(e.Body)
	put(uint64(simplecache.FinalMagic), uint32(1), uint32(0), uint32(0), uint32(0))
	out.Write(e.Stream0)
	out.Write(make([]byte, 32))
	put(uint64(simplecache.FinalMagic), uint32(1|simplecache.FlagHasKeySHA256), uint32(0), uint32(len(e.Stream0)), uint32(0))
	return out.Bytes()
}
