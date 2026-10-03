package sitestorage

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/jsonvalue"
	"github.com/chipskein/cade/internal/v8value"
)

// TextValue is a stored text as a value tree: parsed when it is JSON, as
// sites keep their state, and a string value otherwise, so a schema still
// reaches it with the path "$".
//
//	value := sitestorage.TextValue(`{"drafts":[]}`) // an object
//	value = sitestorage.TextValue("dark")           // the string "dark"
func TextValue(text string) *v8value.Value {
	if value, err := jsonvalue.Parse([]byte(text)); err == nil {
		return value
	}
	return &v8value.Value{Kind: v8value.KindString, Text: text}
}

// UTF16LE decodes text a browser stored as UTF-16 little-endian.
//
//	text, err := sitestorage.UTF16LE([]byte{'o', 0, 'i', 0}) // "oi"
func UTF16LE(raw []byte) (string, error) {
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("UTF-16 string of %d bytes, expected an even length", len(raw))
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units)), nil
}
