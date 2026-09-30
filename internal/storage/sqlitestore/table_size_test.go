package sqlitestore

import (
	"strings"
	"testing"
)

// otherTables groups what is too small to decide on: settings, schema
// versioning and forgotten UIDs.
const otherTables = "other"

// spaceTables are the groups the size benchmark reports (#40), in the
// order docs/BENCHMARKS.md lists them.
var spaceTables = []string{"events", "chunks", "chunks_fts", "chunk_embeddings", "file_modifications", "event_people", otherTables}

// virtualTables keep their data in shadow tables named "<table>_<part>",
// which dbstat lists apart from the table they belong to.
var virtualTables = []string{"chunks_fts", "chunk_embeddings"}

// spaceTableOf maps a table name from sqlite_schema.tbl_name (an index's
// tbl_name is already its table) to the group it counts towards.
func spaceTableOf(tableName string) string {
	for _, virtual := range virtualTables {
		if strings.HasPrefix(tableName, virtual+"_") {
			return virtual
		}
	}
	for _, group := range spaceTables {
		if tableName == group {
			return group
		}
	}
	return otherTables
}

func TestSpaceTableOf(t *testing.T) {
	cases := map[string]string{
		"events":                           "events",
		"chunks":                           "chunks",
		"chunks_fts":                       "chunks_fts",
		"chunks_fts_data":                  "chunks_fts",
		"chunk_embeddings_vector_chunks00": "chunk_embeddings",
		"file_modifications":               "file_modifications",
		"event_people":                     "event_people",
		"forgotten_events":                 otherTables,
		"sqlite_schema":                    otherTables,
	}
	for tableName, want := range cases {
		if got := spaceTableOf(tableName); got != want {
			t.Errorf("spaceTableOf(%q) = %q, want %q", tableName, got, want)
		}
	}
}
