package listing_test

import (
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/testfakes"
)

func uidsOf(events []event.Event) []string {
	var uids []string
	for _, ev := range events {
		uids = append(uids, ev.UID)
	}
	return uids
}

func TestPersonNamesListsEveryFullNameATypedNameMatches(t *testing.T) {
	if got := listing.PersonNames(testfakes.PersonCorpus(), "Ana"); !slices.Equal(got, []string{"ana prado", "ana souza"}) {
		t.Fatalf("expected both Anas (the alias counts as Ana Prado), got %v", got)
	}
	if got := listing.PersonNames(testfakes.PersonCorpus(), "ana prado"); !slices.Equal(got, []string{"ana prado"}) {
		t.Fatalf("expected only Ana Prado, got %v", got)
	}
}

func TestEventsOfPersonTakesHerMessagesMentionsOneToOneChatAndAlias(t *testing.T) {
	got := uidsOf(listing.EventsOfPerson(testfakes.PersonCorpus(), "ana prado"))
	want := []string{"ana-1to1", "me-1to1", "ana-group", "caio-mention", "alias"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
