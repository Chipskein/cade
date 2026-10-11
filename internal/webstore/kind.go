// Package webstore is what schema ingestion reads from: every browser
// storage a web application keeps messages in (IndexedDB, localStorage,
// the HTTP cache, …) is read by a Reader into the same Records, so the
// schema, its discovery and the collector never depend on which storage
// the messages came from.
package webstore

import (
	"fmt"
	"slices"
	"strings"
)

// Kind names one browser storage; a schema's records.kind picks the
// reader that reads it.
type Kind string

const (
	KindIndexedDB    Kind = "indexeddb"
	KindLocalStorage Kind = "local_storage"
	KindOPFS         Kind = "opfs"
	KindHTTPCache    Kind = "http_cache"
	KindCacheAPI     Kind = "cache_api"
)

// knownKinds are the kinds a schema may name, readers or not yet: a schema
// for a storage this build cannot read fails at ingest, naming the kind.
var knownKinds = []Kind{KindIndexedDB, KindLocalStorage, KindOPFS, KindHTTPCache, KindCacheAPI}

// Validate reports a kind no storage has, e.g. a typo in a schema file.
//
//	err := webstore.Kind("local_storage").Validate() // nil
func (k Kind) Validate() error {
	if slices.Contains(knownKinds, k) {
		return nil
	}
	return fmt.Errorf("storage kind %q, expected one of %s", k, joinKinds(knownKinds))
}

func joinKinds(kinds []Kind) string {
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = string(kind)
	}
	return strings.Join(names, ", ")
}
