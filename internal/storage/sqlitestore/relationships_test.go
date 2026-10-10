package sqlitestore

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

// storedRelations lists every relation as "uid from relation to", sorted.
func storedRelations(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT events.uid || ' ' || origin.entity_key || ' ' || relationships.relation || ' ' || target.entity_key
		FROM relationships JOIN events ON events.id = relationships.source_event_id
		JOIN entities origin ON origin.id = relationships.from_entity_id JOIN entities target ON target.id = relationships.to_entity_id ORDER BY 1`)
	testcheck.NoError(t, err)
	values, err := collectStrings(rows)
	testcheck.NoError(t, err)
	return values
}

// commitsToOneFile are two authors' commits to the same file, the shape of
// the retrieval case #21 waits for.
func commitsToOneFile() []event.Event {
	commit := func(uid, author string, offset time.Duration, files ...string) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime.Add(offset), Content: "commit " + uid,
			Metadata: event.Commit{Repository: "/src/api", Hash: uid, Author: author, Files: files}.Metadata()}
	}
	return []event.Event{commit("rui", "Rui Costa", 0, "erp/retry.go"), commit("carla", "Carla Dias", time.Hour, "erp/retry.go", "erp/backoff.go")}
}

func storeWithCommits(t *testing.T) *Store {
	t.Helper()
	store := openTestStore(t)
	for _, ev := range commitsToOneFile() {
		mustSave(t, store, ev, nil)
	}
	return store
}

func TestSaveEventStoresTheRelationsItStates(t *testing.T) {
	store := storeWithCommits(t)
	want := []string{
		"carla carla dias changed /src/api/erp/backoff.go",
		"carla carla dias changed /src/api/erp/retry.go",
		"rui rui costa changed /src/api/erp/retry.go",
	}
	if got := storedRelations(t, store.db); !slices.Equal(got, want) {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEditReplacesTheEventsRelations(t *testing.T) {
	store := storeWithCommits(t)
	ev := commitsToOneFile()[1]
	ev.Metadata = event.Commit{Repository: "/src/api", Hash: "carla", Author: "Carla Dias", Files: []string{"erp/backoff.go"}}.Metadata()
	testcheck.NoError(t, store.UpdateEvent(context.Background(), ev, whole(ev, nil)))
	want := []string{"carla carla dias changed /src/api/erp/backoff.go", "rui rui costa changed /src/api/erp/retry.go"}
	if got := storedRelations(t, store.db); !slices.Equal(got, want) {
		t.Fatalf("expected %q after the edit, got %q", want, got)
	}
}

func TestForgetDeletesTheRelationsOfTheEvent(t *testing.T) {
	store := storeWithCommits(t)
	_, err := store.DeleteEvent(context.Background(), "carla")
	testcheck.NoError(t, err)
	want := []string{"rui rui costa changed /src/api/erp/retry.go"}
	if got := storedRelations(t, store.db); !slices.Equal(got, want) || slices.Contains(entityNames(t, store), "file /src/api/erp/backoff.go") {
		t.Fatalf("expected only Rui's relation and backoff.go gone, got %q and %q", got, entityNames(t, store))
	}
}

func TestRelationshipMigrationRelatesStoredEventsLikeIngestion(t *testing.T) {
	store := storeWithCommits(t)
	want := storedRelations(t, store.db)
	for _, statement := range []string{`DROP TABLE relationships`, `DELETE FROM event_entities WHERE role = 'changed'`, `PRAGMA user_version = 15`} {
		_, err := store.db.Exec(statement)
		testcheck.NoError(t, err)
	}
	testcheck.NoError(t, migrate(context.Background(), store.db, "", schemaMigrations, Hooks{}))
	if got := storedRelations(t, store.db); !slices.Equal(got, want) || versionOf(t, store.db) != latestVersion() {
		t.Fatalf("expected the migrated relations %q to equal a new database's %q", got, want)
	}
}
