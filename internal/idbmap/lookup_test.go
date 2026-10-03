package idbmap

import (
	"errors"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

func TestBuildLookupIndexesOnlyItsStore(t *testing.T) {
	lookup := Lookup{Location: Location{NamespacePrefix: "model-storage", Container: "contact"}, KeyPath: "$.author", Match: "$.id", Values: []Path{"$.name", "$.pushname"}}
	records := []indexeddb.Record{
		record("model-storage", "contact", obj("id", str("1@c.us"), "name", str("Ana"))),
		record("model-storage", "contact", obj("id", str("2@c.us"))),
		record("model-storage", "contact", obj("id", str("4@c.us"), "pushname", str(" Bia "))),
		record("model-storage", "chat", obj("id", str("3@c.us"), "name", str("Grupo"))),
		{Namespace: "model-storage", Container: "contact", DecodeErr: errors.New("corrupt")},
	}
	index, err := buildLookup(lookup, records)
	if err != nil || len(index.values) != 2 {
		t.Fatalf("expected Ana and Bia indexed, got %v (err %v)", index.values, err)
	}
	if got := index.findByPath(obj("author", str("1@c.us"))); got != "Ana" {
		t.Errorf("expected Ana, got %q", got)
	}
	if got := index.findByPath(obj("author", str("4@c.us"))); got != "Bia" {
		t.Errorf("expected the trimmed second value, got %q", got)
	}
	if got := index.findByPath(obj("author", str("3@c.us"))); got != "" {
		t.Errorf("expected nothing from another store, got %q", got)
	}
}

func TestBuildLookupRejectsBadPaths(t *testing.T) {
	if _, err := buildLookup(Lookup{Location: Location{Container: "s"}, KeyPath: "x", Match: "$.id", Values: []Path{"$.name"}}, nil); err == nil {
		t.Fatal("expected an error for a relative key path")
	}
}

func TestConditions(t *testing.T) {
	conditions, err := compileConditions([]Condition{
		{Path: "$.type", In: []string{"chat"}},
		{Path: "$.thread", NotIn: []string{"notifications"}},
		{Path: "$.deleted", KindNot: "object"},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := map[string]struct {
		item *v8value.Value
		keep bool
	}{
		"plain chat":         {obj("type", str("chat")), true},
		"other type":         {obj("type", str("call")), false},
		"notification":       {obj("type", str("chat"), "thread", str("notifications")), false},
		"deleted":            {obj("type", str("chat"), "deleted", obj()), false},
		"deleted flag false": {obj("type", str("chat"), "deleted", &v8value.Value{Kind: v8value.KindBool}), true},
	}
	for name, tc := range cases {
		if got := keepsAll(conditions, tc.item); got != tc.keep {
			t.Errorf("%s: keep = %v, want %v", name, got, tc.keep)
		}
	}
}

// A missing value reads as undefined, so kind_not undefined requires the
// field to exist.
func TestConditionKindNotUndefinedRequiresTheField(t *testing.T) {
	conditions, _ := compileConditions([]Condition{{Path: "$.body", KindNot: "undefined"}})
	if keepsAll(conditions, obj()) || !keepsAll(conditions, obj("body", str("x"))) {
		t.Fatal("expected a missing field to fail kind_not undefined")
	}
}

func TestCompileConditionsRejectsBadInput(t *testing.T) {
	if _, err := compileConditions([]Condition{{Path: "type"}}); err == nil {
		t.Error("relative path: expected an error")
	}
	if _, err := compileConditions([]Condition{{Path: "$.type", KindNot: "objeto"}}); err == nil {
		t.Error("unknown kind: expected an error")
	}
}
