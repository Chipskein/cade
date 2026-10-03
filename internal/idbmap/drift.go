package idbmap

import (
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// PathShape is one path a schema reads, in its store, with the kinds it
// held when the schema was made: the structure, never a value.
type PathShape struct {
	DatabasePrefix string   `json:"database_prefix"`
	Store          string   `json:"store"`
	Path           Path     `json:"path"`
	Kinds          []string `json:"kinds"`
}

// TakeFingerprint records the shape of every path schema reads, as found in
// records, so a later change in the application can be told apart from a
// schema that never matched.
//
//	schema.Fingerprint = idbmap.TakeFingerprint(schema, records)
func TakeFingerprint(schema Schema, records []indexeddb.Record) []PathShape {
	shapes := usedPaths(schema)
	observed := observedKinds(shapes, records)
	for i := range shapes {
		shapes[i].Kinds = observed[shapes[i].key()]
	}
	return shapes
}

// shapeKey names one path of one store, the way the schema selects it.
type shapeKey struct {
	databasePrefix, store string
	path                  Path
}

func (s PathShape) key() shapeKey {
	return shapeKey{s.DatabasePrefix, s.Store, s.Path}
}

// usedPaths lists the store and record path of everything schema reads.
func usedPaths(schema Schema) []PathShape {
	store := storeRef{schema.Records.DatabasePrefix, schema.Records.Store}
	each := schema.Records.Each
	var used usedPathList
	used.add(store, "", each)
	for _, condition := range schema.Require {
		used.add(store, each, condition.Path)
	}
	for _, field := range sortedFields() {
		if rule, exists := schema.Fields[field]; exists {
			used.addRule(store, each, rule)
		}
	}
	return used
}

// storeRef is a store as a schema selects it.
type storeRef struct {
	databasePrefix, store string
}

// usedPathList keeps each store path once, in first-use order.
type usedPathList []PathShape

func (u *usedPathList) addRule(store storeRef, each Path, rule FieldRule) {
	u.add(store, each, rule.Paths...)
	if rule.Lookup == nil {
		return
	}
	u.add(store, each, rule.Lookup.KeyPath)
	u.add(storeRef{rule.Lookup.DatabasePrefix, rule.Lookup.Store}, "", append([]Path{rule.Lookup.Match}, rule.Lookup.Values...)...)
}

func (u *usedPathList) add(store storeRef, root Path, paths ...Path) {
	for _, path := range paths {
		shape := PathShape{DatabasePrefix: store.databasePrefix, Store: store.store, Path: underRoot(root, path)}
		if path != "" && !slices.ContainsFunc(*u, func(used PathShape) bool { return used.key() == shape.key() }) {
			*u = append(*u, shape)
		}
	}
}

// underRoot writes an item path from the record: "$.content" under
// "$.messageMap.<id>" is "$.messageMap.<id>.content".
func underRoot(root, path Path) Path {
	if root == "" {
		return path
	}
	return root + Path(strings.TrimPrefix(string(path), pathRoot))
}

// observedKinds maps each shape's path to the kinds seen there now, sorted,
// in the summary's notation; undefined and null say nothing and are left
// out. Records are selected per store the way the schema selects them.
func observedKinds(shapes []PathShape, records []indexeddb.Record) map[shapeKey][]string {
	kinds := map[shapeKey][]string{}
	for store := range storesOf(shapes) {
		addKinds(kinds, store, idbschema.Summarize(recordsOf(records, store)))
	}
	return kinds
}

func recordsOf(records []indexeddb.Record, store storeRef) []indexeddb.Record {
	var selected []indexeddb.Record
	for _, record := range records {
		if inStore(record, store.databasePrefix, store.store) {
			selected = append(selected, record)
		}
	}
	return selected
}

func storesOf(shapes []PathShape) map[storeRef]bool {
	stores := map[storeRef]bool{}
	for _, shape := range shapes {
		stores[storeRef{shape.DatabasePrefix, shape.Store}] = true
	}
	return stores
}

func addKinds(kinds map[shapeKey][]string, store storeRef, summaries []idbschema.StoreSummary) {
	for _, summary := range summaries {
		for _, field := range summary.Fields {
			key := shapeKey{store.databasePrefix, store.store, Path(field.Path)}
			kinds[key] = mergeKinds(kinds[key], field.Kinds)
		}
	}
}

func mergeKinds(names []string, counts map[v8value.Kind]int) []string {
	for kind := range counts {
		if name := kind.String(); kind != v8value.KindUndefined && kind != v8value.KindNull && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Drift is how far an application's IndexedDB moved from a schema.
type Drift struct {
	Missing []PathShape // paths the schema reads that no record has now
	Changed []PathShape // paths whose kinds share none with the fingerprint, with the kinds seen now
	Tally   Tally
	Format  error // Tally.Check: the store is gone or nothing maps
}

// Drifted reports any change that needs the schema regenerated.
func (d Drift) Drifted() bool {
	return len(d.Missing) > 0 || len(d.Changed) > 0 || d.Format != nil
}

// MeasureDrift compares schema's fingerprint and mapping with records. A
// schema without a fingerprint (written by hand) is judged by its mapping
// alone.
func MeasureDrift(schema Schema, records []indexeddb.Record) (Drift, error) {
	mapper, err := NewMapper(schema)
	if err != nil {
		return Drift{}, err
	}
	_, tally, err := mapper.Apply(records, "")
	if err != nil {
		return Drift{}, err
	}
	drift := Drift{Tally: tally, Format: tally.Check(schema)}
	observed := observedKinds(schema.Fingerprint, records)
	for _, shape := range schema.Fingerprint {
		drift.classify(shape, observed[shape.key()])
	}
	return drift, nil
}

func (d *Drift) classify(shape PathShape, now []string) {
	switch {
	case len(now) == 0 && len(shape.Kinds) > 0:
		d.Missing = append(d.Missing, shape)
	case len(shape.Kinds) > 0 && !slices.ContainsFunc(now, func(kind string) bool { return slices.Contains(shape.Kinds, kind) }):
		shape.Kinds = now
		d.Changed = append(d.Changed, shape)
	}
}
