// Package gguf reads string-valued metadata keys from a GGUF file's
// header, without loading its tensors.
package gguf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// valueType is a GGUF metadata value's type tag, as the format defines it.
type valueType uint32

const (
	typeUint8 valueType = iota
	typeInt8
	typeUint16
	typeInt16
	typeUint32
	typeInt32
	typeFloat32
	typeBool
	typeString
	typeArray
	typeUint64
	typeInt64
	typeFloat64
)

// fixedByteSizes is how many bytes to skip for each type that isn't a
// string or an array.
var fixedByteSizes = map[valueType]int64{
	typeUint8: 1, typeInt8: 1, typeBool: 1,
	typeUint16: 2, typeInt16: 2,
	typeUint32: 4, typeInt32: 4, typeFloat32: 4,
	typeUint64: 8, typeInt64: 8, typeFloat64: 8,
}

var magic = [4]byte{'G', 'G', 'U', 'F'}

// ReadStringMetadata reads the requested string-valued keys from r, a
// GGUF file's bytes read from the start. A key absent from the file, or
// present with a non-string value, is left out of the result.
//
//	metadata, err := gguf.ReadStringMetadata(file, "general.size_label")
func ReadStringMetadata(r io.Reader, keys ...string) (map[string]string, error) {
	br := bufio.NewReader(r)
	if err := checkMagic(br); err != nil {
		return nil, err
	}
	kvCount, err := readHeaderCounts(br)
	if err != nil {
		return nil, err
	}
	wanted := toSet(keys)
	found := make(map[string]string, len(keys))
	for i := uint64(0); i < kvCount && len(found) < len(wanted); i++ {
		if err := readOneEntry(br, wanted, found); err != nil {
			return nil, err
		}
	}
	return found, nil
}

func toSet(keys []string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}
	return set
}

func checkMagic(r io.Reader) error {
	var got [4]byte
	if _, err := io.ReadFull(r, got[:]); err != nil {
		return fmt.Errorf("read gguf magic: %w", err)
	}
	if got != magic {
		return fmt.Errorf("not a gguf file: magic %q, expected %q", got, magic)
	}
	return nil
}

// readHeaderCounts reads past the version and tensor count, returning the
// metadata entry count that follows them.
func readHeaderCounts(r io.Reader) (uint64, error) {
	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return 0, fmt.Errorf("read gguf version: %w", err)
	}
	var tensorCount, kvCount uint64
	if err := binary.Read(r, binary.LittleEndian, &tensorCount); err != nil {
		return 0, fmt.Errorf("read gguf tensor count: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &kvCount); err != nil {
		return 0, fmt.Errorf("read gguf metadata count: %w", err)
	}
	return kvCount, nil
}

// readOneEntry reads one metadata key/value pair, keeping it in found
// only when its key is wanted and its value is a string.
func readOneEntry(r io.Reader, wanted map[string]bool, found map[string]string) error {
	key, err := readString(r)
	if err != nil {
		return fmt.Errorf("read gguf metadata key: %w", err)
	}
	var kind uint32
	if err := binary.Read(r, binary.LittleEndian, &kind); err != nil {
		return fmt.Errorf("read gguf value type for key %q: %w", key, err)
	}
	if valueType(kind) == typeString && wanted[key] {
		value, err := readString(r)
		if err != nil {
			return fmt.Errorf("read gguf string value for key %q: %w", key, err)
		}
		found[key] = value
		return nil
	}
	return skipValue(r, valueType(kind))
}

func readString(r io.Reader) (string, error) {
	var length uint64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func skipValue(r io.Reader, kind valueType) error {
	if kind == typeString {
		_, err := readString(r)
		return err
	}
	if kind == typeArray {
		return skipArray(r)
	}
	size, known := fixedByteSizes[kind]
	if !known {
		return fmt.Errorf("read gguf value: unknown type %d", kind)
	}
	_, err := io.CopyN(io.Discard, r, size)
	return err
}

// skipArray discards an array value: an element type, a length, then that
// many values of that type (recursing for nested arrays).
func skipArray(r io.Reader) error {
	var elementKind uint32
	if err := binary.Read(r, binary.LittleEndian, &elementKind); err != nil {
		return fmt.Errorf("read gguf array element type: %w", err)
	}
	var count uint64
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return fmt.Errorf("read gguf array length: %w", err)
	}
	for i := uint64(0); i < count; i++ {
		if err := skipValue(r, valueType(elementKind)); err != nil {
			return err
		}
	}
	return nil
}
