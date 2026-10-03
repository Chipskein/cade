package idbmap

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
)

// Value builders shared by the package's tests.
func str(text string) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindString, Text: text}
}

func num(number float64) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindNumber, Number: number}
}

func obj(pairs ...any) *v8value.Value {
	object := &v8value.Value{Kind: v8value.KindObject}
	for i := 0; i+1 < len(pairs); i += 2 {
		object.Properties = append(object.Properties, v8value.Property{Key: pairs[i].(string), Value: pairs[i+1].(*v8value.Value)})
	}
	return object
}

func texts(values []*v8value.Value) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = scalarText(value)
	}
	return strings.Join(parts, ",")
}

func mustCompile(t *testing.T, path Path) CompiledPath {
	t.Helper()
	compiled, err := CompilePath(path)
	if err != nil {
		t.Fatalf("compile %q: %v", path, err)
	}
	return compiled
}

func TestResolveProperties(t *testing.T) {
	root := obj("author", obj("_serialized", str("5511@c.us")), "t", num(1727280000))
	if got := texts(mustCompile(t, "$.author._serialized").Resolve(root)); got != "5511@c.us" {
		t.Errorf("nested property: got %q", got)
	}
	if got := mustCompile(t, "$").First(root); got != root {
		t.Error("$ must resolve to the value itself")
	}
	if got := mustCompile(t, "$.missing.deeper").Resolve(root); len(got) != 0 {
		t.Errorf("missing path: expected nothing, got %v", got)
	}
}

// The summary prints message-map keys as <id>; the same path must reach
// every message, and only the data-like keys.
func TestResolveMaskedKeysMatchLikeTheSummary(t *testing.T) {
	root := obj("messageMap", obj(
		"1727280000000", obj("content", str("a")),
		"8:orgid:x", obj("content", str("b")),
		"version", obj("content", str("not a message")),
	))
	if got := texts(mustCompile(t, "$.messageMap.<id>.content").Resolve(root)); got != "a,b" {
		t.Fatalf("expected a,b, got %q", got)
	}
}

func TestResolveContainers(t *testing.T) {
	array := &v8value.Value{Kind: v8value.KindArray, Items: []*v8value.Value{str("x"), nil, str("y")}}
	set := &v8value.Value{Kind: v8value.KindSet, Items: []*v8value.Value{str("s")}}
	mapping := &v8value.Value{Kind: v8value.KindMap, Entries: []v8value.MapEntry{{Key: str("k"), Value: str("v")}}}
	root := obj("list", array, "uniq", set, "lookup", mapping)
	cases := map[Path]string{"$.list[]": "x,y", "$.uniq<>": "s", "$.lookup{}": "v", "$.list<>": "", "$.uniq[]": ""}
	for path, want := range cases {
		if got := texts(mustCompile(t, path).Resolve(root)); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

func TestCompilePathRejectsMalformedPaths(t *testing.T) {
	for _, path := range []Path{"", "content", "$content", "$.", "$..a"} {
		if _, err := CompilePath(path); err == nil {
			t.Errorf("%q: expected an error", path)
		}
	}
}

func TestCompilePathErrorNamesThePath(t *testing.T) {
	_, err := CompilePath("$x")
	if err == nil || !strings.Contains(err.Error(), `"$x"`) {
		t.Fatalf("expected the error to quote the path, got %v", err)
	}
}

func TestParseKind(t *testing.T) {
	if kind, err := parseKind("object"); err != nil || kind != v8value.KindObject {
		t.Errorf("object: got %v, %v", kind, err)
	}
	if kind, err := parseKind(""); err != nil || kind != noKind {
		t.Errorf("empty: got %v, %v", kind, err)
	}
	if _, err := parseKind("objeto"); err == nil {
		t.Error("objeto: expected an error")
	}
}
