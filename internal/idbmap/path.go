package idbmap

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/v8value"
)

// Path is a location inside a decoded value, in the notation the
// idbschema summary prints: "$" is the value itself, ".key" a property,
// "[]" every array element, "{}" every map value, "<>" every set member.
// A masked key (".<id>", ".<text>") matches every property the summary
// would mask the same way, so a summary path is always a valid Path.
// "^" starts a path at the record holding the item instead of the item
// (#78: ChatGPT keeps the time and title on the conversation only).
//
//	"$.messageMap.<id>.content"
//	"^.updateTime"
type Path string

// ReadsRecord reports a path that starts at the record holding the item.
func (p Path) ReadsRecord() bool {
	return strings.HasPrefix(string(p), pathRecordRoot)
}

// FromRecord writes a path of the record as its items read it: "$.title"
// becomes "^.title".
func (p Path) FromRecord() Path {
	return pathRecordRoot + Path(strings.TrimPrefix(string(p), pathRoot))
}

// ReachesOne reports a valid path with no wildcard: it reaches at most one
// value wherever it is read.
func (p Path) ReachesOne() bool {
	compiled, err := CompilePath(p)
	if err != nil {
		return false
	}
	return !slices.ContainsFunc(compiled.steps, func(step pathStep) bool { return step.kind != stepProperty })
}

// Path syntax, shared with idbschema's walk.
const (
	pathRoot       = "$"
	pathRecordRoot = "^"
	pathProperty   = "."
	pathArrayItems = "[]"
	pathMapValues  = "{}"
	pathSetMembers = "<>"
	maskedKeyOpen  = "<"
)

type stepKind int

const (
	stepProperty stepKind = iota
	stepMaskedProperty
	stepArrayItems
	stepMapValues
	stepSetMembers
)

var containerSteps = map[string]stepKind{pathArrayItems: stepArrayItems, pathMapValues: stepMapValues, pathSetMembers: stepSetMembers}

type pathStep struct {
	kind stepKind
	key  string
}

// CompiledPath is a parsed Path, resolved many times while applying a
// schema.
type CompiledPath struct {
	steps      []pathStep
	fromRecord bool
}

// scopedItem is one item a schema maps with the record it was found in;
// both are the same value when the schema has no each.
type scopedItem struct {
	value, record *v8value.Value
}

// FirstIn returns the first value the path reaches from the item, or from
// its record when the path starts with "^".
func (c CompiledPath) FirstIn(item scopedItem) *v8value.Value {
	if c.fromRecord {
		return c.First(item.record)
	}
	return c.First(item.value)
}

// CompilePath parses path.
//
//	compiled, err := idbmap.CompilePath("$.author._serialized")
func CompilePath(path Path) (CompiledPath, error) {
	rest, found := strings.CutPrefix(string(path), pathRoot)
	if path.ReadsRecord() {
		rest, found = strings.CutPrefix(string(path), pathRecordRoot)
	}
	if !found {
		return CompiledPath{}, fmt.Errorf("path %q, expected it to start with %q (the item) or %q (the record holding it)", path, pathRoot, pathRecordRoot)
	}
	compiled := CompiledPath{fromRecord: path.ReadsRecord()}
	for rest != "" {
		step, remaining, err := nextStep(rest)
		if err != nil {
			return CompiledPath{}, fmt.Errorf("path %q: %w", path, err)
		}
		compiled.steps, rest = append(compiled.steps, step), remaining
	}
	return compiled, nil
}

func nextStep(rest string) (pathStep, string, error) {
	for token, kind := range containerSteps {
		if after, found := strings.CutPrefix(rest, token); found {
			return pathStep{kind: kind}, after, nil
		}
	}
	key, found := strings.CutPrefix(rest, pathProperty)
	if !found {
		return pathStep{}, "", fmt.Errorf("unexpected %q, expected %q, %q, %q or %q", rest, pathProperty, pathArrayItems, pathMapValues, pathSetMembers)
	}
	end := keyEnd(key)
	if end == 0 {
		return pathStep{}, "", fmt.Errorf("empty property name before %q", key)
	}
	return propertyStep(key[:end]), key[end:], nil
}

// keyEnd finds where a property name stops: at the next step marker. A
// masked key ("<id>") is read whole, as it starts with the set marker's
// first character.
func keyEnd(key string) int {
	if strings.HasPrefix(key, maskedKeyOpen) {
		if closing := strings.Index(key, ">"); closing > 0 {
			return closing + 1
		}
	}
	end := len(key)
	for _, marker := range []string{pathProperty, pathArrayItems, pathMapValues, pathSetMembers} {
		if index := strings.Index(key, marker); index >= 0 && index < end {
			end = index
		}
	}
	return end
}

func propertyStep(key string) pathStep {
	if strings.HasPrefix(key, maskedKeyOpen) {
		return pathStep{kind: stepMaskedProperty, key: key}
	}
	return pathStep{kind: stepProperty, key: key}
}

// Resolve returns every value the path reaches under root, in order. A
// path without wildcards reaches at most one.
func (c CompiledPath) Resolve(root *v8value.Value) []*v8value.Value {
	current := []*v8value.Value{root}
	for _, step := range c.steps {
		var next []*v8value.Value
		for _, value := range current {
			next = append(next, step.children(value)...)
		}
		current = next
	}
	return nonNil(current)
}

// First returns the first value the path reaches, or nil.
func (c CompiledPath) First(root *v8value.Value) *v8value.Value {
	if found := c.Resolve(root); len(found) > 0 {
		return found[0]
	}
	return nil
}

func (s pathStep) children(value *v8value.Value) []*v8value.Value {
	if value == nil {
		return nil
	}
	switch s.kind {
	case stepProperty:
		return []*v8value.Value{value.Get(s.key)}
	case stepMaskedProperty:
		return maskedProperties(value, s.key)
	case stepMapValues:
		return mapValues(value)
	}
	return itemsOf(value, s.kind)
}

func maskedProperties(value *v8value.Value, mask string) []*v8value.Value {
	var found []*v8value.Value
	for _, property := range value.Properties {
		if idbschema.MaskKey(property.Key) == mask {
			found = append(found, property.Value)
		}
	}
	return found
}

func mapValues(value *v8value.Value) []*v8value.Value {
	values := make([]*v8value.Value, len(value.Entries))
	for i, entry := range value.Entries {
		values[i] = entry.Value
	}
	return values
}

// itemsOf returns array elements or set members, matching the step to the
// value's kind as the summary does.
func itemsOf(value *v8value.Value, kind stepKind) []*v8value.Value {
	isSet := value.Kind == v8value.KindSet
	if isSet != (kind == stepSetMembers) {
		return nil
	}
	return value.Items
}

func nonNil(values []*v8value.Value) []*v8value.Value {
	kept := values[:0]
	for _, value := range values {
		if value != nil {
			kept = append(kept, value)
		}
	}
	return kept
}

// noKind is what an empty kind name parses to: no kind is excluded.
const noKind v8value.Kind = -1

// parseKind reads a kind name as the summary prints it ("object",
// "string"...); "" means no kind.
func parseKind(name string) (v8value.Kind, error) {
	if name == "" {
		return noKind, nil
	}
	for kind := v8value.KindUndefined; kind <= v8value.KindRegExp; kind++ {
		if kind.String() == name {
			return kind, nil
		}
	}
	return noKind, fmt.Errorf("kind %q, expected a value kind such as object, string or number", name)
}
