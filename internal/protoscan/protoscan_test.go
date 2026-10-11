package protoscan

import (
	"encoding/binary"
	"strings"
	"testing"
)

// message encodes fields the way protoc does, for the tests.
type message []byte

func (m message) varint(number, value uint64) message {
	m = binary.AppendUvarint(m, number<<tagTypeBits|uint64(WireVarint))
	return binary.AppendUvarint(m, value)
}

func (m message) bytes(number uint64, value []byte) message {
	m = binary.AppendUvarint(m, number<<tagTypeBits|uint64(WireBytes))
	m = binary.AppendUvarint(m, uint64(len(value)))
	return append(m, value...)
}

func (m message) fixed32(number uint64, value uint32) message {
	m = binary.AppendUvarint(m, number<<tagTypeBits|uint64(WireFixed32))
	return binary.LittleEndian.AppendUint32(m, value)
}

func TestParseReadsEveryWireType(t *testing.T) {
	nested := message{}.bytes(1, []byte("chat"))
	raw := message{}.bytes(1, nested).varint(3, 300).fixed32(4, 7).bytes(2, []byte("https://discord.com"))
	fields, err := Parse(raw)
	if err != nil || len(fields) != 4 {
		t.Fatalf("Parse = %+v, %v; want four fields", fields, err)
	}
	if fields[1].Varint != 300 || binary.LittleEndian.Uint32(fields[2].Bytes) != 7 || String(fields, 2) != "https://discord.com" {
		t.Fatalf("fields = %+v; want the varint, the fixed32 and the string", fields)
	}
	inner, err := Parse(Bytes(fields, 1)[0])
	if err != nil || String(inner, 1) != "chat" {
		t.Fatalf("nested = %+v, %v; want the nested string", inner, err)
	}
}

func TestBytesListsARepeatedField(t *testing.T) {
	fields, err := Parse(message{}.bytes(1, []byte("a")).varint(1, 9).bytes(1, []byte("b")))
	if err != nil {
		t.Fatal(err)
	}
	if values := Bytes(fields, 1); len(values) != 2 || string(values[1]) != "b" {
		t.Fatalf("Bytes = %q; want the two strings, not the varint", values)
	}
	if String(fields, 5) != "" {
		t.Fatal("String of an absent field should be empty")
	}
}

func TestParseRefusesTruncatedFields(t *testing.T) {
	whole := message{}.bytes(1, []byte("discord"))
	for _, raw := range [][]byte{whole[:len(whole)-2], {0x08}, {0x0d, 1, 2}, {0x0b}} {
		if _, err := Parse(raw); err == nil || !strings.Contains(err.Error(), "protobuf field") {
			t.Errorf("Parse(% x) error = %v; want a truncated field reported", raw, err)
		}
	}
}
