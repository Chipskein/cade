// Package simplecache reads the entry files of Chromium's simple disk cache
// backend, which both Chromium request caches use on Linux: the HTTP cache
// (Cache/Cache_Data) and each Cache API cache (Service Worker/CacheStorage).
package simplecache

import (
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
)

// Layout of a <hash>_0 file (net/disk_cache/simple/simple_entry_format.h):
// header, key, stream 1, its EOF record, stream 0, the key's SHA-256 when
// flagged, stream 0's EOF record. Numbers are little-endian.
const (
	InitialMagic = 0xfcfb6d1ba7725c30
	FinalMagic   = 0xf4fa6f45970d41d8
	// Both records are padded to 8 bytes: header magic, version, key
	// length, key hash; EOF magic, flags, CRC-32, stream size.
	HeaderSize      = 24
	EOFSize         = 24
	keyLengthOffset = 12
	eofFlagsOffset  = 8
	eofSizeOffset   = 16
	// FlagHasKeySHA256 marks a SHA-256 of the key before stream 0's EOF.
	FlagHasKeySHA256 = 2
	keySHA256Size    = 32
)

// EntryFilePattern names the files that hold streams 0 and 1; _1 files
// (stream 2) and _s files (sparse ranges) never hold a response body.
var EntryFilePattern = regexp.MustCompile(`^[0-9a-f]{16}_0$`)

// Entry is one entry file whose key has been read; its streams are read
// only when asked for.
type Entry struct {
	Key  string
	file io.ReaderAt
	size int64
}

// OpenEntry reads the header and the key of file, nothing more: a caller
// decides from the key whether the response is read at all.
//
//	entry, err := simplecache.OpenEntry(file, info.Size())
func OpenEntry(file io.ReaderAt, size int64) (Entry, error) {
	header, err := readAt(file, 0, HeaderSize, size)
	if err != nil {
		return Entry{}, err
	}
	if magic := binary.LittleEndian.Uint64(header); magic != InitialMagic {
		return Entry{}, fmt.Errorf("simple cache magic %#x, expected %#x", magic, uint64(InitialMagic))
	}
	key, err := readAt(file, HeaderSize, int64(binary.LittleEndian.Uint32(header[keyLengthOffset:])), size)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Key: string(key), file: file, size: size}, nil
}

// Streams reads stream 0 (the response's headers, in a format each cache
// chooses) and stream 1 (the body as the cache keeps it).
func (e Entry) Streams() (headers, body []byte, err error) {
	stream0End, stream0Size, err := e.stream0Bounds()
	if err != nil {
		return nil, nil, err
	}
	headers, err = readAt(e.file, stream0End-stream0Size, stream0Size, e.size)
	if err != nil {
		return nil, nil, err
	}
	// Stream 1's EOF leaves its size 0: the body is what lies between the
	// key and that record.
	bodyStart := HeaderSize + int64(len(e.Key))
	body, err = readAt(e.file, bodyStart, stream0End-stream0Size-EOFSize-bodyStart, e.size)
	return headers, body, err
}

// stream0Bounds reads stream 0's EOF record at the end of the file.
func (e Entry) stream0Bounds() (end, size int64, err error) {
	eof, err := readAt(e.file, e.size-EOFSize, EOFSize, e.size)
	if err != nil {
		return 0, 0, err
	}
	if magic := binary.LittleEndian.Uint64(eof); magic != FinalMagic {
		return 0, 0, fmt.Errorf("stream 0 EOF magic %#x, expected %#x", magic, uint64(FinalMagic))
	}
	end = e.size - EOFSize
	if binary.LittleEndian.Uint32(eof[eofFlagsOffset:])&FlagHasKeySHA256 != 0 {
		end -= keySHA256Size
	}
	return end, int64(binary.LittleEndian.Uint32(eof[eofSizeOffset:])), nil
}

// readAt reads length bytes at offset, refusing a range outside the file
// (a truncated or corrupt entry).
func readAt(file io.ReaderAt, offset, length, size int64) ([]byte, error) {
	if offset < 0 || length < 0 || offset+length > size {
		return nil, fmt.Errorf("simple cache range of %d bytes at %d, expected it inside the %d-byte entry", length, offset, size)
	}
	raw := make([]byte, length)
	if _, err := file.ReadAt(raw, offset); err != nil {
		return nil, fmt.Errorf("read %d bytes at %d of a simple cache entry: %w", length, offset, err)
	}
	return raw, nil
}
