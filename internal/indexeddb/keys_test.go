package indexeddb

import (
	"encoding/binary"
	"testing"

	"github.com/chipskein/cade/internal/leveldbraw"
)

func TestDecodeKeyPrefixOneByteIDs(t *testing.T) {
	prefix, rest, err := decodeKeyPrefix([]byte{0x00, 3, 7, 1, 0xAA})
	if err != nil || prefix != (keyPrefix{3, 7, 1}) || len(rest) != 1 {
		t.Fatalf("unexpected prefix %+v rest %x (err %v)", prefix, rest, err)
	}
}

func TestDecodeKeyPrefixWideIDs(t *testing.T) {
	// database id width 2, store id width 3, index id width 1.
	first := byte(1<<5 | 2<<2 | 0)
	prefix, _, err := decodeKeyPrefix([]byte{first, 0x34, 0x12, 0x01, 0x00, 0x01, 1})
	if err != nil || prefix.databaseID != 0x1234 || prefix.objectStoreID != 0x010001 {
		t.Fatalf("unexpected prefix %+v (err %v)", prefix, err)
	}
}

func TestDecodeKeyPrefixTruncated(t *testing.T) {
	if _, _, err := decodeKeyPrefix([]byte{0x00, 1}); err == nil {
		t.Fatal("expected an error for a truncated prefix")
	}
}

func TestDecodeKeyPrefixEmpty(t *testing.T) {
	if _, _, err := decodeKeyPrefix(nil); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}

func TestLittleEndianInt(t *testing.T) {
	if got := littleEndianInt([]byte{0x01, 0x02}); got != 0x0201 {
		t.Fatalf("expected 0x0201, got %#x", got)
	}
}

func stringWithLength(text string) []byte {
	encoded := binary.AppendUvarint(nil, uint64(len([]rune(text))))
	for _, char := range text {
		encoded = append(encoded, byte(char>>8), byte(char))
	}
	return encoded
}

func TestDecodeStringWithLength(t *testing.T) {
	text, rest, err := decodeStringWithLength(append(stringWithLength("chats"), 0xAA))
	if err != nil || text != "chats" || len(rest) != 1 {
		t.Fatalf("expected chats, got %q rest %x (err %v)", text, rest, err)
	}
}

func TestDecodeStringWithLengthOverrun(t *testing.T) {
	if _, _, err := decodeStringWithLength([]byte{5, 0, 'a'}); err == nil {
		t.Fatal("expected an error when the string overruns the data")
	}
}

func TestDecodeUTF16BESurrogatePair(t *testing.T) {
	if got := decodeUTF16BE([]byte{0xD8, 0x3C, 0xDF, 0x89}); got != "🎉" {
		t.Fatalf("expected 🎉, got %q", got)
	}
}

func TestBuildCatalog(t *testing.T) {
	databaseKey := append([]byte{0x00, 0, 0, 0, databaseNameTypeByte}, append(stringWithLength("origin"), stringWithLength("chatdb")...)...)
	storeName := stringWithLength("messages")[1:]
	names := buildCatalog([]leveldbraw.Entry{
		{Key: databaseKey, Value: []byte{4}},
		{Key: []byte{0x00, 4, 0, 0, objectStoreMetaTypeByte, 9, objectStoreNameMetaType}, Value: storeName},
		{Key: []byte{0x00, 4, 0, 0, objectStoreMetaTypeByte, 9, 1}, Value: []byte{0, 1}},
	})
	if names.databases[4] != "chatdb" || names.stores[storeRef{4, 9}] != "messages" || len(names.stores) != 1 {
		t.Fatalf("unexpected catalog %+v", names)
	}
}
