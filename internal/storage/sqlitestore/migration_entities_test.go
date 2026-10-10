package sqlitestore

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// storeAtVersion14 saves the events of peopleAcrossSources at path and
// leaves the database as version 14 left it, without the entity tables.
// It returns the links a new database holds for those events.
func storeAtVersion14(t *testing.T, path string) []string {
	t.Helper()
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	defer store.Close()
	for _, ev := range peopleAcrossSources() {
		mustSave(t, store, ev, nil)
	}
	links := storedLinks(t, store.db)
	for _, statement := range []string{`DROP TABLE relationships`, `DROP TABLE event_entities`, `DROP TABLE entities`, `PRAGMA user_version = 14`} {
		_, err := store.db.Exec(statement)
		testcheck.NoError(t, err)
	}
	return links
}

func TestEntityMigrationLinksStoredEventsLikeIngestion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	want := storeAtVersion14(t, path)
	var backup string
	store, err := OpenWithHooks(context.Background(), path, Hooks{BackupCreated: func(p string) { backup = p }})
	testcheck.NoError(t, err)
	defer store.Close()
	if got := storedLinks(t, store.db); !slices.Equal(got, want) || len(want) == 0 || versionOf(t, store.db) != latestVersion() {
		t.Fatalf("expected the migrated links %q to equal a new database's %q at v%d, got v%d", got, want, latestVersion(), versionOf(t, store.db))
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("expected a backup before migration 15, got %q (%v)", backup, err)
	}
}

func TestEntityMigrationRebuildsWhenRunAgain(t *testing.T) {
	store := openTestStore(t)
	for _, ev := range peopleAcrossSources() {
		mustSave(t, store, ev, nil)
	}
	want := storedLinks(t, store.db)
	tx, err := store.db.Begin()
	testcheck.NoError(t, err)
	defer tx.Rollback()
	testcheck.NoError(t, linkStoredEvents(context.Background(), tx))
	testcheck.NoError(t, tx.Commit())
	if got := storedLinks(t, store.db); !slices.Equal(got, want) {
		t.Fatalf("expected the same links after migrating again, got %q, want %q", got, want)
	}
}
