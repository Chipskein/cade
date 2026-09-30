package gguf

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// buildFile assembles a minimal GGUF byte stream with the given
// string-valued metadata entries, for tests only.
func buildFile(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(magic[:])
	writeUint32(&buf, 3)
	writeUint64(&buf, 0)
	writeUint64(&buf, uint64(len(entries)))
	for key, value := range entries {
		writeString(&buf, key)
		writeUint32(&buf, uint32(typeString))
		writeString(&buf, value)
	}
	return buf.Bytes()
}

func writeUint32(buf *bytes.Buffer, v uint32) { _ = binary.Write(buf, binary.LittleEndian, v) }
func writeUint64(buf *bytes.Buffer, v uint64) { _ = binary.Write(buf, binary.LittleEndian, v) }

func writeString(buf *bytes.Buffer, s string) {
	writeUint64(buf, uint64(len(s)))
	buf.WriteString(s)
}

func TestReadStringMetadataFindsRequestedKey(t *testing.T) {
	file := buildFile(t, map[string]string{"general.size_label": "4B", "general.architecture": "qwen3"})
	got, err := ReadStringMetadata(bytes.NewReader(file), "general.size_label")
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if got["general.size_label"] != "4B" {
		t.Fatalf("expected size_label 4B, got %+v", got)
	}
}

func TestReadStringMetadataOmitsAbsentKey(t *testing.T) {
	file := buildFile(t, map[string]string{"general.architecture": "clip"})
	got, err := ReadStringMetadata(bytes.NewReader(file), "general.size_label")
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if _, found := got["general.size_label"]; found {
		t.Fatalf("expected no size_label, got %+v", got)
	}
}

func TestReadStringMetadataSkipsNonStringValues(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(magic[:])
	writeUint32(&buf, 3)
	writeUint64(&buf, 0)
	writeUint64(&buf, 2)
	writeString(&buf, "general.file_type")
	writeUint32(&buf, uint32(typeUint32))
	writeUint32(&buf, 7)
	writeString(&buf, "general.size_label")
	writeUint32(&buf, uint32(typeString))
	writeString(&buf, "2B")

	got, err := ReadStringMetadata(bytes.NewReader(buf.Bytes()), "general.size_label")
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if got["general.size_label"] != "2B" {
		t.Fatalf("expected size_label 2B after skipping a non-string value, got %+v", got)
	}
}

func TestReadStringMetadataRejectsBadMagic(t *testing.T) {
	_, err := ReadStringMetadata(strings.NewReader("nope"))
	if err == nil {
		t.Fatal("expected an error for a non-GGUF file")
	}
}
