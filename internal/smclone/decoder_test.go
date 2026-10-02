package smclone

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/v8value"
)

// clone assembles a structured clone buffer word by word, the way
// SpiderMonkey's SCOutput writes it, so tests read like the format.
type clone struct {
	bytes []byte
}

func newClone() *clone {
	return (&clone{}).pair(tagHeader, 0)
}

func (c *clone) word(value uint64) *clone {
	c.bytes = binary.LittleEndian.AppendUint64(c.bytes, value)
	return c
}

func (c *clone) pair(tag, data uint32) *clone {
	return c.word(uint64(tag)<<32 | uint64(data))
}

func (c *clone) double(number float64) *clone {
	return c.word(math.Float64bits(number))
}

// padded appends raw bytes rounded up to a whole word with zeroes.
func (c *clone) padded(raw []byte) *clone {
	c.bytes = append(c.bytes, raw...)
	for len(c.bytes)%wordSize != 0 {
		c.bytes = append(c.bytes, 0)
	}
	return c
}

// latin1 writes one byte per character; text must stay within U+00FF.
func (c *clone) latin1(text string) *clone {
	var raw []byte
	for _, char := range text {
		raw = append(raw, byte(char))
	}
	return c.pair(tagString, uint32(len(raw))|stringLatin1Flag).padded(raw)
}

func (c *clone) twoByte(text string) *clone {
	units := utf16.Encode([]rune(text))
	raw := make([]byte, 0, 2*len(units))
	for _, unit := range units {
		raw = binary.LittleEndian.AppendUint16(raw, unit)
	}
	return c.pair(tagString, uint32(len(units))).padded(raw)
}

func (c *clone) int32(number int32) *clone {
	return c.pair(tagInt32, uint32(number))
}

func (c *clone) end() *clone {
	return c.pair(tagEndOfKeys, 0)
}

// lengthPrefixed appends a uint32 length and the text, each padded, as
// IndexedDB writes a Blob's type or a File's name.
func (c *clone) lengthPrefixed(text string) *clone {
	return c.padded(binary.LittleEndian.AppendUint32(nil, uint32(len(text)))).padded([]byte(text))
}

func mustDecode(t *testing.T, c *clone) *v8value.Value {
	t.Helper()
	value, err := Decode(c.bytes)
	if err != nil {
		t.Fatalf("decode %x: %v", c.bytes, err)
	}
	return value
}

func TestDecodePrimitives(t *testing.T) {
	cases := []struct {
		name  string
		clone *clone
		want  v8value.Value
	}{
		{"null", newClone().pair(tagNull, 0), v8value.Value{Kind: v8value.KindNull}},
		{"undefined", newClone().pair(tagUndefined, 0), v8value.Value{Kind: v8value.KindUndefined}},
		{"true", newClone().pair(tagBoolean, 1), v8value.Value{Kind: v8value.KindBool, Bool: true}},
		{"negative int32", newClone().int32(-7), v8value.Value{Kind: v8value.KindNumber, Number: -7}},
		{"double", newClone().double(1727280000000.5), v8value.Value{Kind: v8value.KindNumber, Number: 1727280000000.5}},
		{"date", newClone().pair(tagDateObject, 0).double(1727280000000), v8value.Value{Kind: v8value.KindDate, Number: 1727280000000}},
		{"number object", newClone().pair(tagNumberObject, 0).double(2.5), v8value.Value{Kind: v8value.KindNumber, Number: 2.5}},
		{"string object", newClone().pair(tagStringObject, 2|stringLatin1Flag).padded([]byte("ok")), v8value.Value{Kind: v8value.KindString, Text: "ok"}},
	}
	for _, tc := range cases {
		got := mustDecode(t, tc.clone)
		if got.Kind != tc.want.Kind || got.Bool != tc.want.Bool || got.Number != tc.want.Number || got.Text != tc.want.Text {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDecodeNegativeInfinityIsADouble(t *testing.T) {
	if got := mustDecode(t, newClone().double(math.Inf(-1))); got.Kind != v8value.KindNumber || !math.IsInf(got.Number, -1) {
		t.Fatalf("expected -Inf, whose high half equals tagFloatMax, got %+v", got)
	}
}

func TestDecodeStringsInBothEncodings(t *testing.T) {
	if got := mustDecode(t, newClone().latin1("revisão às 15h")).Text; got != "revisão às 15h" {
		t.Errorf("latin1: got %q", got)
	}
	if got := mustDecode(t, newClone().twoByte("ção ✓ 🎉")).Text; got != "ção ✓ 🎉" {
		t.Errorf("two-byte: got %q", got)
	}
}

func TestDecodeBigInt(t *testing.T) {
	positive := newClone().pair(tagBigInt, 2).word(0).word(1)
	if got := mustDecode(t, positive).Text; got != "18446744073709551616" {
		t.Errorf("expected 2^64, got %s", got)
	}
	negative := newClone().pair(tagBigInt, 1|bigIntNegativeFlag).word(42)
	if got := mustDecode(t, negative).Text; got != "-42" {
		t.Errorf("expected -42, got %s", got)
	}
}

func TestDecodeRegExpKeepsSource(t *testing.T) {
	got := mustDecode(t, newClone().pair(tagRegExpObject, 0).latin1("^a+$"))
	if got.Kind != v8value.KindRegExp || got.Text != "^a+$" {
		t.Fatalf("expected regexp ^a+$, got %+v", got)
	}
}

func TestDecodeWithoutHeader(t *testing.T) {
	headless := (&clone{}).latin1("x")
	if got := mustDecode(t, headless).Text; got != "x" {
		t.Fatalf("expected x, got %q", got)
	}
}

func TestDecodeRejectsCorruptInput(t *testing.T) {
	cases := map[string]*clone{
		"empty":               {},
		"unknown tag":         newClone().pair(0xFFFF800A, 0),
		"truncated string":    newClone().pair(tagString, 100|stringLatin1Flag).padded([]byte("short")),
		"huge bigint":         newClone().pair(tagBigInt, 1000),
		"regexp without text": newClone().pair(tagRegExpObject, 0).int32(1),
		"bad back-reference":  newClone().pair(tagBackReferenceObject, 3),
		"unterminated object": newClone().pair(tagObjectObject, 0).latin1("k"),
	}
	for name, c := range cases {
		if _, err := Decode(c.bytes); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDecodeUnknownTagNamesTheTag(t *testing.T) {
	_, err := Decode(newClone().pair(0xFFFF800A, 0).bytes)
	if err == nil || !strings.Contains(err.Error(), "0xffff800a") {
		t.Fatalf("expected the error to name tag 0xffff800a, got %v", err)
	}
}

func TestDecodeBoundsNesting(t *testing.T) {
	deep := newClone()
	for range maxNesting + 1 {
		deep.pair(tagArrayObject, 1).int32(0)
	}
	if _, err := Decode(deep.bytes); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("expected a nesting error, got %v", err)
	}
}
