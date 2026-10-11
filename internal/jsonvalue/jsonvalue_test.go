package jsonvalue

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
)

func TestParseBuildsTheTree(t *testing.T) {
	value, err := Parse([]byte(`[{"id":"9","content":"olá","pinned":false,"edited":null,"type":0,"author":{"name":"ana"}}]`))
	if err != nil || value.Kind != v8value.KindArray || len(value.Items) != 1 {
		t.Fatalf("Parse = %+v, %v; want an array of one object", value, err)
	}
	message := value.Items[0]
	if message.Get("content").String() != "olá" || message.Get("author").Get("name").String() != "ana" {
		t.Fatalf("message = %+v; want content and author.name", message)
	}
	if message.Get("pinned").Kind != v8value.KindBool || message.Get("edited").Kind != v8value.KindNull || message.Get("type").Kind != v8value.KindNumber {
		t.Fatalf("message = %+v; want bool, null and number kinds", message)
	}
}

func TestParseKeepsKeyOrder(t *testing.T) {
	value, err := Parse([]byte(`{"z":1,"a":2,"m":3}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var keys []string
	for _, property := range value.Properties {
		keys = append(keys, property.Key)
	}
	if strings.Join(keys, ",") != "z,a,m" {
		t.Fatalf("keys = %v; want the document's order", keys)
	}
}

func TestParseReadsEmptyContainers(t *testing.T) {
	value, err := Parse([]byte(`{"items":[],"meta":{}}`))
	if err != nil || value.Get("items").Kind != v8value.KindArray || value.Get("meta").Kind != v8value.KindObject {
		t.Fatalf("Parse = %+v, %v; want an empty array and object", value, err)
	}
}

func TestParseRefusesInvalidJSON(t *testing.T) {
	for _, raw := range []string{`{"a":`, `<html>`, `[1] [2]`, ``} {
		if _, err := Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), "bytes of JSON") {
			t.Errorf("Parse(%q) error = %v; want one naming the size", raw, err)
		}
	}
}
