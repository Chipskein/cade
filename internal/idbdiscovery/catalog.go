// Package idbdiscovery proposes an idbmap.Schema for an application's
// IndexedDB with the local model, acting as a translator: the model reads
// the stores' paths and a few masked samples, and fills a fixed target
// under a grammar built from those same paths, so it cannot invent one.
package idbdiscovery

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// CatalogLimits bound what the model reads, so the prompt fits a small
// model's context.
type CatalogLimits struct {
	Stores         int
	PathsPerStore  int
	SamplesPerPath int
	SampledRecords int
}

// DefaultCatalogLimits keep the store prompt near 2,000 tokens, a quarter
// of the generation model's default context of 8,192.
var DefaultCatalogLimits = CatalogLimits{Stores: 12, PathsPerStore: 40, SamplesPerPath: 2, SampledRecords: 200}

// storeLabelPrefix names stores for the model ("S1"); the label maps back
// to the real names, which the model never needs to spell.
const storeLabelPrefix = "S"

// Catalog is what the model sees of one IndexedDB: its largest stores,
// labelled, with their most frequent paths. It holds masked samples only.
type Catalog struct {
	Stores []StoreView
}

// StoreView is one store. Kind, Database and Store are the real storage
// and names, used in the schema; MaskedDatabase is what the model reads.
type StoreView struct {
	Label          string
	Kind           webstore.Kind
	Database       string
	MaskedDatabase string
	Store          string
	Records        int
	Paths          []PathView
}

// PathView is one path with its kinds, most frequent first, and samples.
type PathView struct {
	Path    idbmap.Path
	Count   int
	Kinds   []v8value.Kind
	Samples []string
}

// BuildCatalog groups records by store and keeps the limits' largest
// stores and most frequent paths.
//
//	catalog := idbdiscovery.BuildCatalog(records, idbdiscovery.DefaultCatalogLimits)
func BuildCatalog(records []indexeddb.Record, limits CatalogLimits) Catalog {
	groups := groupByStore(records)
	slices.SortStableFunc(groups, func(a, b storeGroup) int { return cmp.Compare(len(b.records), len(a.records)) })
	var catalog Catalog
	for _, group := range groups[:min(len(groups), limits.Stores)] {
		view := group.view(limits)
		if len(view.Paths) == 0 {
			continue
		}
		view.Label = fmt.Sprintf("%s%d", storeLabelPrefix, len(catalog.Stores)+1)
		catalog.Stores = append(catalog.Stores, view)
	}
	return catalog
}

// Store returns the store labelled label.
func (c Catalog) Store(label string) (StoreView, error) {
	for _, store := range c.Stores {
		if store.Label == label {
			return store, nil
		}
	}
	return StoreView{}, fmt.Errorf("store %q, expected one of the catalog's %d labels", label, len(c.Stores))
}

type storeGroup struct {
	kind            webstore.Kind
	database, store string
	records         []indexeddb.Record
}

// groupByStore keeps decoded records only, in first-seen store order.
func groupByStore(records []indexeddb.Record) []storeGroup {
	index := map[[2]string]int{}
	var groups []storeGroup
	for _, record := range records {
		if record.DecodeErr != nil {
			continue
		}
		key := [2]string{record.Namespace, record.Container}
		if _, seen := index[key]; !seen {
			index[key] = len(groups)
			groups = append(groups, storeGroup{kind: record.Kind, database: record.Namespace, store: record.Container})
		}
		groups[index[key]].records = append(groups[index[key]].records, record)
	}
	return groups
}

func (g storeGroup) view(limits CatalogLimits) StoreView {
	view := StoreView{Kind: g.kind, Database: g.database, MaskedDatabase: idbschema.MaskName(g.database), Store: g.store, Records: len(g.records)}
	for _, summary := range idbschema.Summarize(g.records) {
		view.Paths = append(view.Paths, frequentPaths(summary.Fields, limits.PathsPerStore)...)
	}
	sampled := g.records[:min(len(g.records), limits.SampledRecords)]
	for i := range view.Paths {
		view.Paths[i].Samples = collectSamples(view.Paths[i].Path, sampled, limits.SamplesPerPath)
	}
	return view
}

// frequentPaths keeps the most frequent paths that ever hold a value
// (always-undefined paths say nothing), in path order.
func frequentPaths(fields []idbschema.FieldStat, limit int) []PathView {
	var paths []PathView
	for _, field := range fields {
		if kinds := valueKinds(field.Kinds); len(kinds) > 0 {
			paths = append(paths, PathView{Path: idbmap.Path(field.Path), Count: field.Count, Kinds: kinds})
		}
	}
	slices.SortStableFunc(paths, func(a, b PathView) int { return cmp.Compare(b.Count, a.Count) })
	paths = paths[:min(len(paths), limit)]
	slices.SortFunc(paths, func(a, b PathView) int { return strings.Compare(string(a.Path), string(b.Path)) })
	return paths
}

// valueKinds lists the kinds seen, most frequent first, without undefined
// and null.
func valueKinds(counts map[v8value.Kind]int) []v8value.Kind {
	var kinds []v8value.Kind
	for kind := range counts {
		if kind != v8value.KindUndefined && kind != v8value.KindNull {
			kinds = append(kinds, kind)
		}
	}
	slices.SortFunc(kinds, func(a, b v8value.Kind) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	return kinds
}

// Under re-roots the store's paths at each: the paths of one item the
// each path selects, written from "$". An empty each keeps every path.
func (s StoreView) Under(each idbmap.Path) []PathView {
	if each == "" {
		return s.Paths
	}
	var items []PathView
	for _, path := range s.Paths {
		rest, found := strings.CutPrefix(string(path.Path), string(each))
		if found && startsAStep(rest) {
			path.Path = idbmap.Path("$" + rest)
			items = append(items, path)
		}
	}
	return items
}

// pathStepStarts are how a step begins in the summary notation; a prefix
// that ends mid-key ("$.a" of "$.ab") is not a parent.
var pathStepStarts = []string{".", "[]", "{}", "<>"}

func startsAStep(rest string) bool {
	for _, start := range pathStepStarts {
		if strings.HasPrefix(rest, start) {
			return true
		}
	}
	return false
}
