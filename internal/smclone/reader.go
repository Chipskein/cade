package smclone

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

// wordSize is the unit of the format: every pair, number and padded byte
// run occupies a multiple of 8 bytes (SCOutput in StructuredClone.cpp).
const wordSize = 8

// pairReader walks the little-endian 64-bit words of a clone buffer.
type pairReader struct {
	data []byte
	pos  int
}

func (r *pairReader) readWord() (uint64, error) {
	if len(r.data)-r.pos < wordSize {
		return 0, fmt.Errorf("need an 8-byte word at offset %d, %d bytes remain", r.pos, len(r.data)-r.pos)
	}
	word := binary.LittleEndian.Uint64(r.data[r.pos:])
	r.pos += wordSize
	return word, nil
}

// peekTag returns the tag of the next pair without consuming it.
func (r *pairReader) peekTag() (uint32, error) {
	word, err := r.readWord()
	if err != nil {
		return 0, err
	}
	r.pos -= wordSize
	return uint32(word >> 32), nil
}

func (r *pairReader) readDouble() (float64, error) {
	word, err := r.readWord()
	return math.Float64frombits(word), err
}

// readPadded reads length bytes and skips the zero padding that rounds
// them up to a whole word.
func (r *pairReader) readPadded(length uint64) ([]byte, error) {
	remaining := uint64(len(r.data) - r.pos)
	if length > remaining {
		return nil, fmt.Errorf("need %d bytes at offset %d, %d remain", length, r.pos, remaining)
	}
	raw := r.data[r.pos : r.pos+int(length)]
	padded := (length + wordSize - 1) / wordSize * wordSize
	r.pos += int(min(padded, remaining))
	return raw, nil
}

func (r *pairReader) readLatin1(length uint64) (string, error) {
	raw, err := r.readPadded(length)
	if err != nil {
		return "", err
	}
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes), nil
}

func (r *pairReader) readTwoByte(length uint64) (string, error) {
	if length > math.MaxUint64/2 {
		return "", fmt.Errorf("two-byte string of %d chars at offset %d is too long", length, r.pos)
	}
	raw, err := r.readPadded(length * 2)
	if err != nil {
		return "", err
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units)), nil
}
