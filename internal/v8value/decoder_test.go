package v8value

import (
	"encoding/binary"
	"math"
	"testing"
)

// v8 prepends the version header V8 writes (format 15).
func v8(body ...byte) []byte {
	return append([]byte{0xFF, 0x0F}, body...)
}

func latin1(text string) []byte {
	return append([]byte{'"', byte(len(text))}, text...)
}

func mustDecode(t *testing.T, payload []byte) *Value {
	t.Helper()
	value, err := Decode(payload)
	if err != nil {
		t.Fatalf("decode %x: %v", payload, err)
	}
	return value
}

func TestDecodePrimitives(t *testing.T) {
	cases := map[string]struct {
		payload []byte
		kind    Kind
	}{
		"undefined": {v8('_'), KindUndefined}, "null": {v8('0'), KindNull},
		"true": {v8('T'), KindBool}, "string": {v8(latin1("hi")...), KindString},
	}
	for name, c := range cases {
		if got := mustDecode(t, c.payload); got.Kind != c.kind {
			t.Errorf("%s: expected %s, got %s", name, c.kind, got.Kind)
		}
	}
}

func TestDecodeInt32ZigZag(t *testing.T) {
	if got := mustDecode(t, v8('I', 0x0D)); got.Number != -7 {
		t.Fatalf("expected -7, got %v", got.Number)
	}
}

func TestDecodeUint32(t *testing.T) {
	if got := mustDecode(t, v8('U', 0xAC, 0x02)); got.Number != 300 {
		t.Fatalf("expected 300, got %v", got.Number)
	}
}

func TestDecodeDoubleAndDate(t *testing.T) {
	raw := binary.LittleEndian.AppendUint64(nil, math.Float64bits(1.5))
	number := mustDecode(t, v8(append([]byte{'N'}, raw...)...))
	date := mustDecode(t, v8(append([]byte{'D'}, raw...)...))
	if number.Number != 1.5 || date.Kind != KindDate || date.Number != 1.5 {
		t.Fatalf("unexpected %+v / %+v", number, date)
	}
}

func TestDecodeBigInt(t *testing.T) {
	// bitfield: 2 bytes (<<1) with sign bit set; magnitude 0x0102 little-endian.
	if got := mustDecode(t, v8('Z', 0x05, 0x02, 0x01)); got.Kind != KindBigInt || got.Text != "-258" {
		t.Fatalf("expected -258, got %+v", got)
	}
}

func TestDecodeLatin1IsNotUTF8(t *testing.T) {
	if got := mustDecode(t, v8('"', 1, 0xE1)); got.Text != "á" {
		t.Fatalf("expected á, got %q", got.Text)
	}
}

func TestDecodeTwoByteString(t *testing.T) {
	if got := mustDecode(t, v8('c', 4, 0x3C, 0xD8, 0x89, 0xDF)); got.Text != "🎉" {
		t.Fatalf("expected 🎉, got %q", got.Text)
	}
}

func TestDecodeTwoByteStringRejectsOddLength(t *testing.T) {
	if _, err := Decode(v8('c', 3, 1, 2, 3)); err == nil {
		t.Fatal("expected an error for an odd byte count")
	}
}

func TestDecodeUTF8String(t *testing.T) {
	if got := mustDecode(t, v8('S', 2, 0xC3, 0xA9)); got.Text != "é" {
		t.Fatalf("expected é, got %q", got.Text)
	}
}

func TestDecodeSkipsPaddingAndVerifyCount(t *testing.T) {
	if got := mustDecode(t, v8(0x00, 0x00, '?', 0x03, 'T')); !got.Bool {
		t.Fatal("expected true after padding and verify tags")
	}
}

func TestDecodeRejectsMissingHeader(t *testing.T) {
	if _, err := Decode([]byte{'T'}); err == nil {
		t.Fatal("expected an error without the version header")
	}
}

func TestDecodeRejectsUnknownTag(t *testing.T) {
	if _, err := Decode(v8('\\', 'b')); err == nil {
		t.Fatal("expected an error for a Blink host object")
	}
}

func TestDecodeRejectsTruncatedData(t *testing.T) {
	if _, err := Decode(v8('"', 10, 'a')); err == nil {
		t.Fatal("expected an error for a truncated string")
	}
}

func TestDecodeRejectsDanglingReference(t *testing.T) {
	if _, err := Decode(v8('^', 5)); err == nil {
		t.Fatal("expected an error for a reference to an unseen object")
	}
}

func TestDecodeRejectsExcessiveNesting(t *testing.T) {
	body := make([]byte, 0, 2*maxNesting+2)
	for range maxNesting + 1 {
		body = append(body, 'A', 1)
	}
	if _, err := Decode(v8(body...)); err == nil {
		t.Fatal("expected an error for nesting beyond the limit")
	}
}

func TestPropertyKeyRejectsObjectKeys(t *testing.T) {
	if _, err := propertyKey(&Value{Kind: KindObject}); err == nil {
		t.Fatal("expected an error for an object used as a key")
	}
}

func TestReversed(t *testing.T) {
	if got := reversed([]byte{1, 2, 3}); got[0] != 3 || got[2] != 1 {
		t.Fatalf("expected [3 2 1], got %v", got)
	}
}
