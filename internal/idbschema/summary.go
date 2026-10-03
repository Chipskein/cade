// Package idbschema summarizes the shape of IndexedDB records — database
// and store names, field paths and value types — without any values, so the
// report can be shared to design an ingestor safely.
package idbschema

import (
	"errors"
	"sort"

	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// maxWalkDepth bounds how deep field paths go; deeper structure adds noise
// and back-references can form cycles.
const maxWalkDepth = 6

// FieldStat counts how often a field path occurred and with which types.
type FieldStat struct {
	Path  string
	Count int
	Kinds map[v8value.Kind]int
}

// StoreSummary describes one object store.
type StoreSummary struct {
	Database    string
	Store       string
	Records     int
	Failed      int
	BlobWrapped int
	Fields      []FieldStat
}

// Summarize groups records by store and collects their masked field paths.
//
//	report := idbschema.Summarize(records)
func Summarize(records []webstore.Record) []StoreSummary {
	builders := map[string]*storeBuilder{}
	var order []string
	for _, record := range records {
		key := record.Namespace + "\x00" + record.Container
		if builders[key] == nil {
			builders[key] = newStoreBuilder(record)
			order = append(order, key)
		}
		builders[key].add(record)
	}
	summaries := make([]StoreSummary, len(order))
	for i, key := range order {
		summaries[i] = builders[key].summary()
	}
	return summaries
}

// storeBuilder accumulates one store's statistics.
type storeBuilder struct {
	result StoreSummary
	fields map[string]*FieldStat
}

func newStoreBuilder(record webstore.Record) *storeBuilder {
	return &storeBuilder{
		result: StoreSummary{Database: MaskName(record.Namespace), Store: MaskName(record.Container)},
		fields: map[string]*FieldStat{},
	}
}

func (b *storeBuilder) add(record webstore.Record) {
	b.result.Records++
	if errors.Is(record.DecodeErr, webstore.ErrExternalValue) {
		b.result.BlobWrapped++
		return
	}
	if record.DecodeErr != nil {
		b.result.Failed++
		return
	}
	walkValue("", record.Value, 0, map[*v8value.Value]bool{}, b.observe)
}

func (b *storeBuilder) observe(path string, kind v8value.Kind) {
	stat := b.fields[path]
	if stat == nil {
		stat = &FieldStat{Path: path, Kinds: map[v8value.Kind]int{}}
		b.fields[path] = stat
	}
	stat.Count++
	stat.Kinds[kind]++
}

// summary returns fields ordered by path, which keeps parents above
// children.
func (b *storeBuilder) summary() StoreSummary {
	result := b.result
	for _, stat := range b.fields {
		result.Fields = append(result.Fields, *stat)
	}
	sort.Slice(result.Fields, func(i, j int) bool { return result.Fields[i].Path < result.Fields[j].Path })
	return result
}
