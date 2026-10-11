package smclone

import (
	"bytes"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
)

func TestDecodeObjectWithStringAndIndexKeys(t *testing.T) {
	object := mustDecode(t, newClone().pair(tagObjectObject, 0).
		latin1("id").latin1("3EB0").
		int32(7).pair(tagBoolean, 1).
		latin1("t").double(1727280000).
		end())
	if object.Kind != v8value.KindObject || object.Get("id").String() != "3EB0" || !object.Get("7").IsTrue() || object.Get("t").Number != 1727280000 {
		t.Fatalf("unexpected object %+v", object)
	}
}

func TestDecodeSparseArrayKeepsHolesAndNamedProperties(t *testing.T) {
	array := mustDecode(t, newClone().pair(tagArrayObject, 3).
		int32(0).latin1("a").
		int32(2).latin1("c").
		latin1("extra").pair(tagNull, 0).
		end())
	if len(array.Items) != 3 || array.Items[0].Text != "a" || array.Items[1] != nil || array.Items[2].Text != "c" {
		t.Fatalf("expected [a, hole, c], got %+v", array.Items)
	}
	if array.Get("extra") == nil || len(array.Properties) != 1 {
		t.Fatalf("expected the named property to stay, got %+v", array.Properties)
	}
}

func TestDecodeArrayCapsDeclaredLength(t *testing.T) {
	array := mustDecode(t, newClone().pair(tagArrayObject, 1<<31).int32(5).latin1("x").end())
	if len(array.Items) != maxSparseItems || array.Items[5].Text != "x" {
		t.Fatalf("expected %d capped items with x at 5, got %d", maxSparseItems, len(array.Items))
	}
}

func TestDecodeMapAndSet(t *testing.T) {
	mapping := mustDecode(t, newClone().pair(tagMapObject, 0).latin1("k").int32(1).int32(2).latin1("v").end())
	if len(mapping.Entries) != 2 || mapping.Entries[0].Key.Text != "k" || mapping.Entries[1].Value.Text != "v" {
		t.Fatalf("unexpected map %+v", mapping.Entries)
	}
	set := mustDecode(t, newClone().pair(tagSetObject, 0).latin1("a").latin1("b").end())
	if set.Kind != v8value.KindSet || len(set.Items) != 2 || set.Items[1].Text != "b" {
		t.Fatalf("unexpected set %+v", set.Items)
	}
}

// Objects are numbered in creation order, the root included; strings are
// not objects and take no slot.
func TestDecodeBackReferencesCountObjectsInCreationOrder(t *testing.T) {
	root := mustDecode(t, newClone().pair(tagObjectObject, 0).
		latin1("author").pair(tagObjectObject, 0).latin1("user").latin1("5511").end().
		latin1("again").pair(tagBackReferenceObject, 1).
		latin1("self").pair(tagBackReferenceObject, 0).
		end())
	if root.Get("again") != root.Get("author") || root.Get("self") != root {
		t.Fatalf("expected references to author (1) and root (0), got %+v", root.Properties)
	}
}

func TestDecodeArrayBuffersAndViews(t *testing.T) {
	raw := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	current := mustDecode(t, newClone().pair(tagArrayBufferObject, 0).word(uint64(len(raw))).padded(raw))
	legacy := mustDecode(t, newClone().pair(tagArrayBufferObjectV2, uint32(len(raw))).padded(raw))
	if !bytes.Equal(current.Bytes, raw) || !bytes.Equal(legacy.Bytes, raw) {
		t.Fatalf("expected the raw bytes from both buffer layouts, got %v and %v", current.Bytes, legacy.Bytes)
	}
	const uint16Type = 3
	view := mustDecode(t, newClone().pair(tagTypedArrayObject, uint16Type).word(2).
		pair(tagArrayBufferObject, 0).word(uint64(len(raw))).padded(raw).word(4))
	if !bytes.Equal(view.Bytes, raw[4:8]) {
		t.Fatalf("expected two uint16 from offset 4, got %v", view.Bytes)
	}
}

func TestDecodeViewThatOverflowsItsBufferFails(t *testing.T) {
	overflow := newClone().pair(tagDataViewObject, 0).word(8).pair(tagArrayBufferObjectV2, 4).padded([]byte{1, 2, 3, 4}).word(0)
	if _, err := Decode(overflow.bytes); err == nil {
		t.Fatal("expected an error for an 8-byte view over a 4-byte buffer")
	}
}

func TestDecodeBlobAndFileDescriptions(t *testing.T) {
	blob := mustDecode(t, newClone().pair(tagDOMBlob, 0).word(2048).lengthPrefixed("image/jpeg"))
	if blob.Get(blobSizeKey).Number != 2048 || blob.Get(blobTypeKey).String() != "image/jpeg" {
		t.Fatalf("unexpected blob %+v", blob.Properties)
	}
	file := mustDecode(t, newClone().pair(tagDOMFile, 1).word(10).lengthPrefixed("text/plain").word(1727280000000).lengthPrefixed("notas.txt"))
	if file.Get(fileNameKey).String() != "notas.txt" || file.Get(fileLastModifiedKey).Number != 1727280000000 {
		t.Fatalf("unexpected file %+v", file.Properties)
	}
}
