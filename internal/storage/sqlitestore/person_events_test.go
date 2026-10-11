package sqlitestore

import (
	"context"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
)

func storeWithPeople(t *testing.T) (*Store, []event.Event) {
	t.Helper()
	store, corpus := openTestStore(t), testfakes.PersonCorpus()
	for _, ev := range corpus {
		mustSave(t, store, ev, []float32{1, 0})
	}
	return store, corpus
}

func TestPersonQueriesSelectWhatTheListingReferenceKeeps(t *testing.T) {
	store, corpus := storeWithPeople(t)
	for _, name := range []string{"Ana", "Ana Prado", "Caio", "Ninguém"} {
		got, err := store.PeopleNamed(context.Background(), name)
		testcheck.NoError(t, err)
		if want := listing.PersonNames(corpus, name); !slices.Equal(got, want) {
			t.Errorf("PeopleNamed(%q) = %v, memory %v", name, got, want)
		}
	}
	for _, key := range []string{"ana prado", "ana souza", "caio mendes"} {
		events, err := store.EventsOfPerson(context.Background(), key)
		testcheck.NoError(t, err)
		got, want := uidsOf(events), uidsOf(listing.EventsOfPerson(corpus, key))
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("EventsOfPerson(%q) = %v, memory %v", key, got, want)
		}
	}
}

// Acceptance (#86): after forgetting her events, nothing of Ana Prado is
// left in the people index, the entities, the text or the keyword index.
func TestForgettingAPersonsEventsLeavesNothingOfTheName(t *testing.T) {
	store, _ := storeWithPeople(t)
	events, err := store.EventsOfPerson(context.Background(), "ana prado")
	testcheck.NoError(t, err)
	for _, ev := range events {
		_, err := store.DeleteEvent(context.Background(), ev.UID)
		testcheck.NoError(t, err)
	}
	var left int
	testcheck.NoError(t, store.db.QueryRow(`SELECT (SELECT COUNT(*) FROM event_people WHERE name LIKE '%prado%')
		+ (SELECT COUNT(*) FROM entities WHERE entity_key LIKE '%prado%')
		+ (SELECT COUNT(*) FROM events WHERE content LIKE '%Prado%' OR metadata LIKE '%Prado%' OR metadata LIKE '%prado%')`).Scan(&left))
	hits, err := store.SearchLexical(context.Background(), storage.LexicalQuery{Match: `"prado" OR "ana"`, Limit: 10})
	if err != nil || left != 0 || len(hits) != 0 {
		t.Fatalf("expected nothing of Ana Prado left, got %d rows and %d keyword hits (err %v)", left, len(hits), err)
	}
}
