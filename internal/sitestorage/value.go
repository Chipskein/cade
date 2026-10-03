package sitestorage

import (
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
