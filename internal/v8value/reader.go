package v8value

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

// byteReader walks the serialized bytes; every error names the offset so a
// failure can be located without printing (possibly private) content.
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("unexpected end of data at offset %d", r.pos)
	}
	value := r.data[r.pos]
	r.pos++
	return value, nil
}

func (r *byteReader) peekByte() (byte, bool) {
	if r.pos >= len(r.data) {
		return 0, false
	}
	return r.data[r.pos], true
}

func (r *byteReader) readBytes(count uint64) ([]byte, error) {
	if count > uint64(len(r.data)-r.pos) {
		return nil, fmt.Errorf("need %d bytes at offset %d, only %d remain", count, r.pos, len(r.data)-r.pos)
	}
	start := r.pos
	r.pos += int(count)
	return r.data[start:r.pos], nil
}

func (r *byteReader) readVarint() (uint64, error) {
	value, width := binary.Uvarint(r.data[r.pos:])
	if width <= 0 {
		return 0, fmt.Errorf("invalid varint at offset %d", r.pos)
	}
	r.pos += width
	return value, nil
}

func (r *byteReader) readZigZag() (int64, error) {
	encoded, err := r.readVarint()
	return int64(encoded>>1) ^ -int64(encoded&1), err
}

func (r *byteReader) readDouble() (float64, error) {
	raw, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(raw)), nil
}

// readLatin1 decodes V8 one-byte strings, which are Latin-1, not UTF-8.
func (r *byteReader) readLatin1() (string, error) {
	raw, err := r.readLengthPrefixed()
	runes := make([]rune, len(raw))
	for i, char := range raw {
		runes[i] = rune(char)
	}
	return string(runes), err
}

func (r *byteReader) readUTF16LE() (string, error) {
	raw, err := r.readLengthPrefixed()
	if err != nil || len(raw)%2 != 0 {
		return "", fmt.Errorf("two-byte string of %d bytes before offset %d, expected an even length: %v", len(raw), r.pos, err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units)), nil
}

func (r *byteReader) readLengthPrefixed() ([]byte, error) {
	length, err := r.readVarint()
	if err != nil {
		return nil, err
	}
	return r.readBytes(length)
}
