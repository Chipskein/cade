// Package snappyblock decodes Snappy block-format data (as used inside
// LevelDB tables and Chromium IndexedDB values). It is the project's only
// dependency point on a Snappy implementation.
package snappyblock

import (
	"fmt"

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
