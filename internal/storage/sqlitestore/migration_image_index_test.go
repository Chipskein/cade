package sqlitestore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The index only helps if lookups use the same expression; the plan shows
// whether SQLite picks it.
func TestImageHashLookupUsesTheIndex(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "cade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var id, parent, unused int
	var detail string
	row := store.db.QueryRow(`EXPLAIN QUERY PLAN SELECT uid FROM events WHERE `+imageSHA256Expression+` = ?`, "ab12")
	if err := row.Scan(&id, &parent, &unused, &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "events_image_sha256") {
		t.Fatalf("expected the image hash index in the plan, got %q", detail)
	}
}
