package sqlitestore

import (
	"context"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

// entityNames lists the stored entities as "type key", sorted.
func entityNames(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.db.Query(`SELECT type || ' ' || entity_key FROM entities ORDER BY 1`)
	testcheck.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		testcheck.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	testcheck.NoError(t, rows.Err())
	return names
}

func storeAcrossSources(t *testing.T) *Store {
	t.Helper()
	store := openTestStore(t)
	for _, ev := range peopleAcrossSources() {
		mustSave(t, store, ev, nil)
	}
	return store
}

func TestForgetKeepsAnEntityOtherEventsReferTo(t *testing.T) {
	store := storeAcrossSources(t)
	_, err := store.DeleteEvent(context.Background(), "m1")
	testcheck.NoError(t, err)
	want := []string{"file /home/eu/plano.md", "person ana sousa", "person ana souza"}
	if got := entityNames(t, store); !slices.Equal(got, want) {
		t.Fatalf("expected Ana Souza kept for her commit, got %q", got)
	}
	_, err = store.DeleteEvent(context.Background(), "c2")
	testcheck.NoError(t, err)
	want = []string{"file /home/eu/plano.md", "person ana sousa"}
	if got := entityNames(t, store); !slices.Equal(got, want) {
		t.Fatalf("expected Ana Souza gone with her last event, got %q", got)
	}
}

func TestForgetFileDeletesTheFileEntity(t *testing.T) {
	store := storeAcrossSources(t)
	_, err := store.DeleteEvent(context.Background(), "f1")
	testcheck.NoError(t, err)
	if got := entityNames(t, store); slices.Contains(got, "file /home/eu/plano.md") {
		t.Fatalf("expected the file entity gone with its event, got %q", got)
	}
}

func TestForgetSourceKeepsEntitiesOfOtherSources(t *testing.T) {
	store := storeAcrossSources(t)
	_, err := store.DeleteSource(context.Background(), event.SourceGit)
	testcheck.NoError(t, err)
	want := []string{"file /home/eu/plano.md", "person ana souza"}
	if got := entityNames(t, store); !slices.Equal(got, want) || len(storedLinks(t, store.db)) != 2 {
		t.Fatalf("expected the Teams sender and the file kept, got %q", got)
	}
}

func TestEditThatDropsANameDeletesItsEntity(t *testing.T) {
	store := storeAcrossSources(t)
	ev := peopleAcrossSources()[1]
	ev.Metadata = event.Commit{Author: "Ana Souza", Email: "ana@example.com"}.Metadata()
	testcheck.NoError(t, store.UpdateEvent(context.Background(), ev, whole(ev, nil)))
	if got := entityNames(t, store); slices.Contains(got, "person ana sousa") {
		t.Fatalf("expected the dropped spelling's entity gone, got %q", got)
	}
}
