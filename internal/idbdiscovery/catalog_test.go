package idbdiscovery

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/firefoxidb"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/smclone"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
	_ "github.com/mattn/go-sqlite3"
)

// Real samples with synthetic data, written by Chrome and Floorp.
const (
	teamsSampleDir   = "../../testdata/teams-formats/2026-09.leveldb"
	firefoxSampleDir = "../../testdata/firefox-indexeddb"
)

func teamsCatalog(t *testing.T) Catalog {
	t.Helper()
	records, err := indexeddb.ReadDirectory(teamsSampleDir)
	if err != nil {
		t.Fatalf("read Teams sample: %v", err)
	}
	return BuildCatalog(records, DefaultCatalogLimits)
}

func firefoxRecords(t *testing.T) []webstore.Record {
	t.Helper()
	open := func(path string) (*sql.DB, error) { return sql.Open("sqlite3", path) }
	records, err := firefoxidb.NewReader(smclone.Decode, open).ReadDirectory(firefoxSampleDir)
	if err != nil {
		t.Fatalf("read Floorp sample: %v", err)
	}
	return records
}

func storeNamed(t *testing.T, catalog Catalog, name string) StoreView {
	t.Helper()
	for _, store := range catalog.Stores {
		if store.Store == name {
			return store
		}
	}
	t.Fatalf("no store %q in %+v", name, catalog.Stores)
	return StoreView{}
}

func pathNamed(store StoreView, path idbmap.Path) (PathView, bool) {
	index := slices.IndexFunc(store.Paths, func(view PathView) bool { return view.Path == path })
	if index < 0 {
		return PathView{}, false
	}
	return store.Paths[index], true
}

func TestBuildCatalogLabelsLargestStoresFirst(t *testing.T) {
	catalog := teamsCatalog(t)
	if len(catalog.Stores) != 3 || catalog.Stores[0].Label != "S1" || catalog.Stores[2].Store != "profiles" {
		t.Fatalf("expected three labelled stores, profiles last, got %+v", catalog.Stores)
	}
	chains := storeNamed(t, catalog, "replychains")
	if !strings.HasPrefix(chains.Database, "Teams:replychain-manager:") || chains.Records != 3 {
		t.Fatalf("expected the real database name and 3 records, got %q / %d", chains.Database, chains.Records)
	}
}

func TestBuildCatalogShowsMaskedSamplesOnly(t *testing.T) {
	chains := storeNamed(t, teamsCatalog(t), "replychains")
	arrival, _ := pathNamed(chains, "$.messageMap.<id>.originalArrivalTime")
	conversation, _ := pathNamed(chains, "$.conversationId")
	if !slices.Equal(arrival.Samples, []string{"<n:13>"}) {
		t.Errorf("expected a number shown by digit count, got %q", arrival.Samples)
	}
	if len(conversation.Samples) == 0 || !strings.Contains(conversation.Samples[0], "<email>") {
		t.Errorf("expected the conversation id masked, got %q", conversation.Samples)
	}
}

func TestBuildCatalogDropsAlwaysEmptyPaths(t *testing.T) {
	message := storeNamed(t, BuildCatalog(firefoxRecords(t), DefaultCatalogLimits), "message")
	if _, found := pathNamed(message, "$.undef"); found {
		t.Fatal("a path that is always undefined says nothing and must be left out")
	}
	body, _ := pathNamed(message, "$.body")
	if !slices.Equal(body.Kinds, []v8value.Kind{v8value.KindString}) {
		t.Fatalf("expected $.body as a string, got %+v", body)
	}
}

func TestBuildCatalogRespectsLimits(t *testing.T) {
	catalog := BuildCatalog(firefoxRecords(t), CatalogLimits{Stores: 1, PathsPerStore: 3, SamplesPerPath: 1, SampledRecords: 1})
	if len(catalog.Stores) != 1 || len(catalog.Stores[0].Paths) != 3 {
		t.Fatalf("expected 1 store with 3 paths, got %+v", catalog.Stores)
	}
}

func TestCatalogStoreByLabel(t *testing.T) {
	catalog := teamsCatalog(t)
	if store, err := catalog.Store("S2"); err != nil || store.Label != "S2" {
		t.Errorf("S2: got %+v (err %v)", store, err)
	}
	if _, err := catalog.Store("S9"); err == nil || !strings.Contains(err.Error(), "S9") {
		t.Errorf("S9: expected an error naming the label, got %v", err)
	}
}

func TestUnderReRootsItemPaths(t *testing.T) {
	store := StoreView{Paths: []PathView{{Path: "$.messageMap.<id>"}, {Path: "$.messageMap.<id>.content"}, {Path: "$.messageMapper"}, {Path: "$.id"}}}
	items := store.Under("$.messageMap.<id>")
	if len(items) != 1 || items[0].Path != "$.content" {
		t.Fatalf("expected only $.content, got %+v", items)
	}
	if len(store.Under("")) != 4 {
		t.Fatal("an empty each keeps every path")
	}
}

func TestRenderSample(t *testing.T) {
	cases := []struct {
		value *v8value.Value
		want  string
	}{
		{&v8value.Value{Kind: v8value.KindString, Text: "ana@corp.com escreveu\nalgo"}, "<email> escreveu algo"},
		{&v8value.Value{Kind: v8value.KindString, Text: strings.Repeat("a", 50)}, strings.Repeat("a", 40) + "…"},
		{&v8value.Value{Kind: v8value.KindNumber, Number: 1727280000}, "<n:10>"},
		{&v8value.Value{Kind: v8value.KindNumber, Number: -7}, "<n:1>"},
		{&v8value.Value{Kind: v8value.KindNumber, Number: 0.5}, "<n:fraction>"},
		{&v8value.Value{Kind: v8value.KindBigInt, Text: "-12345"}, "<n:5>"},
		{&v8value.Value{Kind: v8value.KindBool, Bool: true}, "true"},
		{&v8value.Value{Kind: v8value.KindDate, Number: 1}, "<date>"},
	}
	for _, tc := range cases {
		if got, ok := renderSample(tc.value); !ok || got != tc.want {
			t.Errorf("renderSample(%+v) = %q, want %q", tc.value, got, tc.want)
		}
	}
	if _, ok := renderSample(&v8value.Value{Kind: v8value.KindObject}); ok {
		t.Error("objects have no sample")
	}
}
