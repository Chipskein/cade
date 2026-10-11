// Package protoscan reads the fields of a protocol buffer message without
// its .proto file: enough for the few Chromium index files cade reads,
// without a dependency on the protobuf runtime.
package protoscan

import (
	"encoding/binary"
	"fmt"
)

// WireType is how a field's value is encoded.
type WireType uint64

const (
	WireVarint  WireType = 0
	WireFixed64 WireType = 1
	WireBytes   WireType = 2
	WireFixed32 WireType = 5
)

// fixedSizes are the sizes of the fixed-width wire types.
var fixedSizes = map[WireType]int{WireFixed64: 8, WireFixed32: 4}

// tagTypeBits is how many low bits of a tag hold its wire type.
const tagTypeBits = 3

// Field is one field of a message: Varint for varints, Bytes for strings,
// nested messages and fixed-width values.
type Field struct {
	Number uint64
	Type   WireType
	Varint uint64
	Bytes  []byte
}

// Parse lists the fields of one message, in order; repeated fields appear
// once per value.
//
//	fields, err := protoscan.Parse(indexFile)
func Parse(raw []byte) ([]Field, error) {
	var fields []Field
	for offset := 0; offset < len(raw); {
		field, size, err := parseField(raw[offset:])
		if err != nil {
			return nil, fmt.Errorf("protobuf field at byte %d of %d: %w", offset, len(raw), err)
		}
		fields = append(fields, field)
		offset += size
	}
	return fields, nil
}

func parseField(raw []byte) (Field, int, error) {
	tag, tagSize := binary.Uvarint(raw)
	if tagSize <= 0 {
		return Field{}, 0, fmt.Errorf("unreadable tag, expected a varint")
	}
	field := Field{Number: tag >> tagTypeBits, Type: WireType(tag & (1<<tagTypeBits - 1))}
	size, err := field.readValue(raw[tagSize:])
	return field, tagSize + size, err
}

func (f *Field) readValue(raw []byte) (int, error) {
	switch f.Type {
	case WireVarint:
		value, size := binary.Uvarint(raw)
		if size <= 0 {
			return 0, fmt.Errorf("field %d: unreadable varint", f.Number)
		}
		f.Varint = value
		return size, nil
	case WireBytes:
		length, size := binary.Uvarint(raw)
		if size <= 0 || length > uint64(len(raw)-size) {
			return 0, fmt.Errorf("field %d: length %d with %d bytes left, expected it to fit", f.Number, length, len(raw)-size)
		}
		f.Bytes = raw[size : size+int(length)]
		return size + int(length), nil
	}
	return f.readFixed(raw)
}

func (f *Field) readFixed(raw []byte) (int, error) {
	size, known := fixedSizes[f.Type]
	if !known {
		return 0, fmt.Errorf("field %d: wire type %d, expected 0, 1, 2 or 5", f.Number, f.Type)
	}
	if len(raw) < size {
		return 0, fmt.Errorf("field %d: %d bytes left, expected %d", f.Number, len(raw), size)
	}
	f.Bytes = raw[:size]
	return size, nil
}

// Bytes lists the length-delimited values of field number, in order:
// the strings or nested messages of a repeated field.
//
//	caches := protoscan.Bytes(fields, 1)
func Bytes(fields []Field, number uint64) [][]byte {
	var values [][]byte
	for _, field := range fields {
		if field.Number == number && field.Type == WireBytes {
			values = append(values, field.Bytes)
		}
	}
	return values
}

// String is the first string of field number, "" when absent.
func String(fields []Field, number uint64) string {
	values := Bytes(fields, number)
	if len(values) == 0 {
		return ""
	}
	return string(values[0])
}
