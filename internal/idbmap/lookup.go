package idbmap

import (
	"strings"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// lookupIndex answers one Lookup: the Value of each record of its store,
// by that record's Match.
type lookupIndex struct {
	keyPath CompiledPath
	values  map[string]string
}

func buildLookup(lookup Lookup, records []indexeddb.Record) (lookupIndex, error) {
	paths, err := compilePaths(lookup.KeyPath, lookup.Match, lookup.Value)
	if err != nil {
		return lookupIndex{}, err
	}
	index := lookupIndex{keyPath: paths[0], values: map[string]string{}}
	for _, record := range records {
		if !inStore(record, lookup.DatabasePrefix, lookup.Store) {
			continue
		}
		key, value := readText(paths[1].First(record.Value), TransformNone), readText(paths[2].First(record.Value), TransformTrim)
		if key != "" && value != "" {
			index.values[key] = value
		}
	}
	return index, nil
}

// find returns the looked-up value for item, or "".
func (l lookupIndex) find(item *v8value.Value) string {
	return l.values[readText(l.keyPath.First(item), TransformNone)]
}

// inStore reports whether record decoded and belongs to the store.
func inStore(record indexeddb.Record, databasePrefix, store string) bool {
	return record.DecodeErr == nil && record.Store == store && strings.HasPrefix(record.Database, databasePrefix)
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
