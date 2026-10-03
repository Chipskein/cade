package idbmap

import (
	"fmt"

	"github.com/chipskein/cade/internal/webstore"
)

// Location is where a schema finds records: Container, in a namespace
// whose name starts with NamespacePrefix (in IndexedDB, the object store of
// a database often named after the user, so only its stable start is
// kept).
//
// V1DatabasePrefix and V1Store are the names version 1 files gave them,
// read so the schemas saved before storages other than IndexedDB keep
// working unedited, moved to the generic names on parse and never written.
type Location struct {
	NamespacePrefix  string `json:"namespace_prefix,omitempty"`
	Container        string `json:"container"`
	V1DatabasePrefix string `json:"database_prefix,omitempty"`
	V1Store          string `json:"store,omitempty"`
}

// selects reports whether record decoded and lives at l.
func (l Location) selects(record webstore.Record) bool {
	return record.InContainer(l.NamespacePrefix, l.Container)
}

// upgradeV1 moves the version 1 names to the generic ones.
func (l *Location) upgradeV1() error {
	if l.NamespacePrefix != "" || l.Container != "" {
		return fmt.Errorf("namespace_prefix %q / container %q in a version %d schema, expected database_prefix and store", l.NamespacePrefix, l.Container, indexedDBOnlyVersion)
	}
	l.NamespacePrefix, l.Container = l.V1DatabasePrefix, l.V1Store
	l.V1DatabasePrefix, l.V1Store = "", ""
	return nil
}

// rejectV1 refuses the version 1 names in a newer schema, where they
// would be silently ignored.
func (l Location) rejectV1() error {
	if l.V1DatabasePrefix != "" || l.V1Store != "" {
		return fmt.Errorf("database_prefix %q / store %q are version %d names, expected namespace_prefix and container", l.V1DatabasePrefix, l.V1Store, indexedDBOnlyVersion)
	}
	return nil
}
