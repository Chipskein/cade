// Package snappyblock decodes Snappy block-format data (as used inside
// LevelDB tables and Chromium IndexedDB values) and framed streams (Firefox
// Cache API bodies). It is the project's only dependency point on a Snappy
// implementation.
package snappyblock

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/snappy"
)

// Decode decompresses one Snappy block.
//
//	plain, err := snappyblock.Decode(compressed)
func Decode(compressed []byte) ([]byte, error) {
	plain, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, fmt.Errorf("decode %d-byte snappy block: %w", len(compressed), err)
	}
	return plain, nil
}

// MaxFramedBytes caps a decoded framed stream: the Cache API bodies cade
// reads are pages of messages, not media.
const MaxFramedBytes = 64 << 20

// DecodeFramed decompresses a Snappy framing-format stream (the
// "sNaPpY" chunks Firefox writes each Cache API body as).
//
//	plain, err := snappyblock.DecodeFramed(bodyFile)
func DecodeFramed(compressed []byte) ([]byte, error) {
	// Firefox's chunk checksums do not match the framing format's masked
	// CRC-32C (every body of a Floorp profile fails it, #75), so they are
	// not checked; a corrupt body still fails to parse as JSON.
	reader := s2.NewReader(bytes.NewReader(compressed), s2.ReaderIgnoreCRC())
	plain, err := io.ReadAll(io.LimitReader(reader, MaxFramedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decode %d-byte snappy stream: %w", len(compressed), err)
	}
	if len(plain) > MaxFramedBytes {
		return nil, fmt.Errorf("snappy stream of %d bytes decodes past %d, expected at most that", len(compressed), MaxFramedBytes)
	}
	return plain, nil
}
