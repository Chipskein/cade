package v8value

import "testing"

func TestDecodeObjectWithNumericKey(t *testing.T) {
	payload := v8(append(append([]byte{'o'}, latin1("a")...), append([]byte{'T', 'I', 0x04}, append(latin1("x"), '{', 2)...)...)...)
	object := mustDecode(t, payload)
	if object.Kind != KindObject || !object.Get("a").Bool || object.Get("2").String() != "x" {
		t.Fatalf("unexpected object %+v", object)
	}
}

func TestDecodeObjectBackReference(t *testing.T) {
	// {inner: {}, again: <ref 1>} — the root is object 0, inner is object 1.
	body := []byte{'o'}
	body = append(append(body, latin1("inner")...), 'o', '{', 0)
	body = append(append(body, latin1("again")...), '^', 1, '{', 2)
	root := mustDecode(t, v8(body...))
	if root.Get("inner") != root.Get("again") {
		t.Fatal("expected the back-reference to resolve to the same value")
	}
}

func TestDecodeDenseArrayWithHole(t *testing.T) {
	array := mustDecode(t, v8('A', 2, 'T', '-', '$', 0, 2))
	if array.Kind != KindArray || len(array.Items) != 2 || array.Items[1] != nil || !array.Items[0].Bool {
		t.Fatalf("unexpected array %+v", array)
	}
}

func TestDecodeSparseArrayPromotesIndexes(t *testing.T) {
	array := mustDecode(t, v8('a', 3, 'I', 0x04, 'T', '@', 1, 3))
	if len(array.Items) != 3 || array.Items[2] == nil || !array.Items[2].Bool || len(array.Properties) != 0 {
		t.Fatalf("unexpected sparse array %+v", array)
	}
}

func TestPromoteIndexPropertiesKeepsNamedProperties(t *testing.T) {
	array := &Value{Properties: []Property{{Key: "0", Value: &Value{}}, {Key: "label", Value: &Value{}}}}
	promoteIndexProperties(array, 1)
	if array.Items[0] == nil || len(array.Properties) != 1 || array.Properties[0].Key != "label" {
		t.Fatalf("unexpected promotion %+v", array)
	}
}

func TestDecodeMap(t *testing.T) {
	mapping := mustDecode(t, v8(append(append([]byte{';'}, latin1("k")...), 'I', 2, ':', 2)...))
	if mapping.Kind != KindMap || len(mapping.Entries) != 1 || mapping.Entries[0].Value.Number != 1 {
		t.Fatalf("unexpected map %+v", mapping)
	}
}

func TestDecodeSet(t *testing.T) {
	set := mustDecode(t, v8('\'', 'T', 'F', ',', 2))
	if set.Kind != KindSet || len(set.Items) != 2 {
		t.Fatalf("unexpected set %+v", set)
	}
}

func TestDecodeBoxedString(t *testing.T) {
	if got := mustDecode(t, v8(append([]byte{'s'}, latin1("x")...)...)); got.String() != "x" {
		t.Fatalf("expected x, got %+v", got)
	}
}

func TestDecodeRegExp(t *testing.T) {
	got := mustDecode(t, v8(append(append([]byte{'R'}, latin1("a+")...), 1)...))
	if got.Kind != KindRegExp || got.Text != "a+" {
		t.Fatalf("unexpected regexp %+v", got)
	}
}

func TestDecodeTypedArrayView(t *testing.T) {
	got := mustDecode(t, v8('B', 4, 1, 2, 3, 4, 'V', 'B', 1, 2, 0))
	if got.Kind != KindBinary || len(got.Bytes) != 2 || got.Bytes[0] != 2 {
		t.Fatalf("expected bytes [2 3], got %+v", got)
	}
}

func TestDecodeTypedArrayViewOutOfRange(t *testing.T) {
	if _, err := Decode(v8('B', 2, 1, 2, 'V', 'B', 1, 5, 0)); err == nil {
		t.Fatal("expected an error for a view past the buffer")
	}
}

func TestDecodeUnterminatedObject(t *testing.T) {
	if _, err := Decode(v8(append([]byte{'o'}, latin1("a")...)...)); err == nil {
		t.Fatal("expected an error for an object without its end tag")
	}
}

func TestKindString(t *testing.T) {
	if KindMap.String() != "map" || Kind(99).String() != "kind(99)" {
		t.Fatalf("unexpected names %q %q", KindMap.String(), Kind(99).String())
	}
}

func TestIsTrue(t *testing.T) {
	var missing *Value
	if missing.IsTrue() || (&Value{Kind: KindString, Bool: true}).IsTrue() || !(&Value{Kind: KindBool, Bool: true}).IsTrue() {
		t.Fatal("expected only a true boolean to be true")
	}
}

func TestGetAndStringOnNil(t *testing.T) {
	var missing *Value
	if missing.Get("x") != nil || missing.String() != "" || (&Value{Kind: KindNumber}).String() != "" {
		t.Fatal("expected nil-safe accessors")
	}
}
