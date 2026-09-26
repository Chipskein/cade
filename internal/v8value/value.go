// Package v8value decodes V8's ValueSerializer format (the structured-clone
// wire format Chromium uses for IndexedDB values) into an explicit tree.
package v8value

import "strconv"

// Kind is the JavaScript type of a Value.
type Kind int

const (
	KindUndefined Kind = iota
	KindNull
	KindBool
	KindNumber
	KindBigInt
	KindString
	KindDate
	KindObject
	KindArray
	KindMap
	KindSet
	KindBinary
	KindRegExp
)

var kindNames = [...]string{"undefined", "null", "bool", "number", "bigint", "string", "date", "object", "array", "map", "set", "binary", "regexp"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "kind(" + strconv.Itoa(int(k)) + ")"
}

// Value is one decoded JavaScript value. Which fields are set depends on
// Kind: Bool; Number (also dates, as epoch milliseconds); Text (strings,
// bigint digits, regexp source); Properties (objects and extra array
// properties); Items (array elements, nil for holes, and set members);
// Entries (maps); Bytes (array buffers and views).
type Value struct {
	Kind       Kind
	Bool       bool
	Number     float64
	Text       string
	Properties []Property
	Items      []*Value
	Entries    []MapEntry
	Bytes      []byte
}

// Property is one own property of an object.
type Property struct {
	Key   string
	Value *Value
}

// MapEntry is one key/value pair of a Map.
type MapEntry struct {
	Key   *Value
	Value *Value
}

// Get returns the property named key, or nil when absent or v is not an
// object.
//
//	content := message.Get("content")
func (v *Value) Get(key string) *Value {
	if v == nil {
		return nil
	}
	for _, property := range v.Properties {
		if property.Key == key {
			return property.Value
		}
	}
	return nil
}

// IsTrue reports whether v is the boolean true; nil and non-booleans are
// false, so absent fields can be read without a nil check.
func (v *Value) IsTrue() bool {
	return v != nil && v.Kind == KindBool && v.Bool
}

// String returns the text of a string value and "" for anything else.
func (v *Value) String() string {
	if v == nil || v.Kind != KindString {
		return ""
	}
	return v.Text
}
