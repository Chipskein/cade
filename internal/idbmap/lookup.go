package idbmap

import (
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// lookupIndex answers one Lookup: the first value of each record of its
// store, by that record's Match.
type lookupIndex struct {
	keyPath  *CompiledPath
	keyField Field
	values   map[string]string
}

func buildLookup(lookup Lookup, records []webstore.Record) (lookupIndex, error) {
	index := lookupIndex{keyField: lookup.KeyField, values: map[string]string{}}
	var err error
	if index.keyPath, err = compileEach(lookup.KeyPath); err != nil {
		return lookupIndex{}, err
	}
	paths, err := compilePaths(append([]Path{lookup.Match}, lookup.Values...)...)
	if err != nil {
		return lookupIndex{}, err
	}
	for _, record := range records {
		if !lookup.selects(record) {
			continue
		}
		if key, value := readText(paths[0].First(record.Value), TransformNone), firstText(paths[1:], record.Value); key != "" && value != "" {
			index.values[key] = value
		}
	}
	return index, nil
}

// firstText is the first of paths with a value under root, trimmed.
func firstText(paths []CompiledPath, root *v8value.Value) string {
	for _, path := range paths {
		if text := readText(path.First(root), TransformTrim); text != "" {
			return text
		}
	}
	return ""
}

// findByPath returns the looked-up value for item's KeyPath, or "".
func (l lookupIndex) findByPath(item *v8value.Value) string {
	if l.keyPath == nil {
		return ""
	}
	return l.values[readText(l.keyPath.First(item), TransformNone)]
}

func compilePaths(paths ...Path) ([]CompiledPath, error) {
	compiled := make([]CompiledPath, len(paths))
	for i, path := range paths {
		var err error
		if compiled[i], err = CompilePath(path); err != nil {
			return nil, err
		}
	}
	return compiled, nil
}
