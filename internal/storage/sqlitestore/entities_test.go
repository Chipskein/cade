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

// storedLinks lists every link as "uid type key name role", in a stable
// order, so tests compare the whole entity layer at once.
func storedLinks(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT events.uid || ' ' || entities.type || ' ' || entities.entity_key || ' ' || entities.canonical_name || ' ' || event_entities.role
		FROM event_entities JOIN events ON events.id = event_entities.event_id JOIN entities ON entities.id = event_entities.entity_id ORDER BY 1`)
	testcheck.NoError(t, err)
	defer rows.Close()
	var links []string
	for rows.Next() {
		var link string
		testcheck.NoError(t, rows.Scan(&link))
		links = append(links, link)
	}
	testcheck.NoError(t, rows.Err())
	return links
}

func entityCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	testcheck.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&count))
	return count
}

// peopleAcrossSources are the same person as a Teams sender and a commit
// author, and a file: what the migration test also stores.
func peopleAcrossSources() []event.Event {
	return []event.Event{
		{UID: "m1", Source: event.SourceTeams, Timestamp: baseTime, Content: "Ana Souza: segue o plano",
			Metadata: event.Message{Sender: "Ana Souza", Conversation: "Ana Souza, Eu"}.Metadata()},
		{UID: "c1", Source: event.SourceGit, Timestamp: baseTime.Add(time.Minute), Content: "Ajusta o plano",
			Metadata: event.Commit{Author: "Ana Sousa", Email: "ana@example.com"}.Metadata()},
		{UID: "c2", Source: event.SourceGit, Timestamp: baseTime.Add(2 * time.Minute), Content: "Revisa o plano",
			Metadata: event.Commit{Author: "Ana Souza", Email: "ana@example.com"}.Metadata()},
		{UID: "f1", Source: event.SourceFile, Timestamp: baseTime.Add(3 * time.Minute), Content: "plano",
			Metadata: event.File{Path: "/home/eu/plano.md"}.Metadata()},
	}
}

func TestSaveEventLinksOneEntityAcrossSources(t *testing.T) {
	store := openTestStore(t)
	for _, ev := range peopleAcrossSources() {
		mustSave(t, store, ev, nil)
	}
	want := []string{
		"c1 person ana sousa ana sousa author",
		"c2 person ana souza ana souza author",
		"f1 file /home/eu/plano.md plano.md subject",
		"m1 person ana souza ana souza sender",
	}
	if got := storedLinks(t, store.db); !slices.Equal(got, want) || entityCount(t, store.db) != 3 {
		t.Fatalf("expected %q over 3 entities, got %q over %d", want, got, entityCount(t, store.db))
	}
}

func TestUpdateEventReplacesItsLinks(t *testing.T) {
	store := openTestStore(t)
	ev := peopleAcrossSources()[0]
	mustSave(t, store, ev, nil)
	ev.Metadata = event.Message{Sender: "Rui Costa", Conversation: "Rui Costa, Eu"}.Metadata()
	testcheck.NoError(t, store.UpdateEvent(context.Background(), ev, whole(ev, nil)))
	want := []string{"m1 person rui costa rui costa sender"}
	if got := storedLinks(t, store.db); !slices.Equal(got, want) {
		t.Fatalf("expected %q after the edit, got %q", want, got)
	}
}

func TestEntityKeepsTheFirstSpellingStored(t *testing.T) {
	store := openTestStore(t)
	first := event.Event{UID: "m1", Source: event.SourceTeams, Timestamp: baseTime, Content: "oi",
		Metadata: event.Message{Sender: "Willian Prado"}.Metadata()}
	second := event.Event{UID: "m2", Source: event.SourceTeams, Timestamp: baseTime.Add(time.Minute), Content: "oi de novo",
		Metadata: event.Message{Sender: "Wilian Prado"}.Metadata()}
	mustSave(t, store, first, nil)
	mustSave(t, store, second, nil)
	want := []string{"m1 person wilian prado willian prado sender", "m2 person wilian prado willian prado sender"}
	if got := storedLinks(t, store.db); !slices.Equal(got, want) {
		t.Fatalf("expected both spellings on one entity named by the first, got %q", got)
	}
}
