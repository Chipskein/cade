package idbmap

import "github.com/chipskein/cade/internal/webstore"

// Location is where a schema finds records: Container, in a namespace
// whose name starts with NamespacePrefix (in IndexedDB, the object store of
// a database often named after the user, so only its stable start is
// kept).
type Location struct {
	NamespacePrefix string `json:"database_prefix"`
	Container       string `json:"store"`
}

// selects reports whether record decoded and lives at l.
func (l Location) selects(record webstore.Record) bool {
	return record.InContainer(l.NamespacePrefix, l.Container)
}
