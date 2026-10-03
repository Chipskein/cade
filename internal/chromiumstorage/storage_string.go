package chromiumstorage

import (
	"fmt"

	"github.com/chipskein/cade/internal/sitestorage"
)

// Chromium's localStorage writes a string as one format byte and its
// characters: UTF-16 little-endian, or Latin-1 when every character fits
// in a byte (dom_storage_database.cc, StorageFormat).
const (
	formatUTF16  = 0
	formatLatin1 = 1
)

// decodeStorageString reads a key or value as localStorage stored it.
func decodeStorageString(stored []byte) (string, error) {
	if len(stored) == 0 {
		return "", fmt.Errorf("empty localStorage string, expected a format byte (%d or %d) first", formatUTF16, formatLatin1)
	}
	switch stored[0] {
	case formatLatin1:
		return latin1(stored[1:]), nil
	case formatUTF16:
		return sitestorage.UTF16LE(stored[1:])
	}
	return "", fmt.Errorf("localStorage string format %d, expected %d (UTF-16) or %d (Latin-1)", stored[0], formatUTF16, formatLatin1)
}

func latin1(raw []byte) string {
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}
