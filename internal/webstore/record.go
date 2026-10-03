package webstore

import (
	"errors"
	"strings"

	"github.com/chipskein/cade/internal/v8value"
)

// Record is one value a browser storage holds, decoded into the same value
// tree whatever the storage. Namespace and Container locate it inside its
// origin, two levels each storage fills its own way: the database and the
// object store in IndexedDB, the cache name and the request URL in the
// Cache API, no namespace and the key in localStorage, no namespace and
// the URL in the HTTP cache.
//
// Key is the value's own key where the storage keys values by text, empty
// where the reader does not decode keys (IndexedDB: a schema reaches a
// message through the paths of its value). Exactly one of Value and
// DecodeErr is set: a record that fails to decode is still reported, so
// callers can count what they could not read instead of silently losing it.
type Record struct {
	Kind      Kind
	Origin    string
	Namespace string
	Container string
	Key       string
	Value     *v8value.Value
	DecodeErr error
	// Database and Store are the IndexedDB names of Namespace and
	// Container, kept only while the tests that build records move to the
	// generic names (#73); the IndexedDB readers fill both until then.
	Database string
	Store    string
}

// ErrExternalValue reports a value kept outside the storage that was read,
// in a file of its own (a Chromium IndexedDB blob, a Firefox external
// structured clone).
var ErrExternalValue = errors.New("value stored in an external file")

// InContainer reports whether the record decoded and lives in container,
// in a namespace starting with namespacePrefix: how a schema selects the
// records that hold messages, whose namespace often ends in a per-user id.
//
//	record.InContainer("Teams:replychain-manager:", "replychains")
func (r Record) InContainer(namespacePrefix, container string) bool {
	namespace, actual := r.Location()
	return r.DecodeErr == nil && actual == container && strings.HasPrefix(namespace, namespacePrefix)
}

// Location is where the record lives: Namespace and Container, or
// Database and Store for a record built with the IndexedDB names (#73,
// until they go).
func (r Record) Location() (namespace, container string) {
	if r.Namespace == "" && r.Container == "" {
		return r.Database, r.Store
	}
	return r.Namespace, r.Container
}

// WithOrigin names the origin of every record, which a reader knows from
// the location rather than from each value.
//
//	return webstore.WithOrigin(records, "https+++web.whatsapp.com"), nil
func WithOrigin(records []Record, origin string) []Record {
	for i := range records {
		records[i].Origin = origin
	}
	return records
}
