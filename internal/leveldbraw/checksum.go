package leveldbraw

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// LevelDB stores CRC32C values "masked" so that a checksum of data that
// itself contains checksums stays well distributed.
const crcMaskDelta = 0xa282ead8

func maskedChecksum(parts ...[]byte) uint32 {
	var crc uint32
	for _, part := range parts {
		crc = crc32.Update(crc, castagnoli, part)
	}
	return ((crc >> 15) | (crc << 17)) + crcMaskDelta
}

func verifyChecksum(stored []byte, what string, parts ...[]byte) error {
	expected := binary.LittleEndian.Uint32(stored)
	if actual := maskedChecksum(parts...); actual != expected {
		return fmt.Errorf("%s checksum %08x, expected %08x", what, actual, expected)
	}
	return nil
}
