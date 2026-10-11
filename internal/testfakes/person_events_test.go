package testfakes

import (
	"context"
	"slices"
	"testing"
)

// An @mention keeps only the first name, so "@Ana" counts for both Anas;
// forget --person shows it in the list before deleting.
func TestFakePersonIndexFollowsTheListingReference(t *testing.T) {
	store := NewFakeEventStore()
	store.Events = PersonCorpus()
	names, _ := store.PeopleNamed(context.Background(), "Ana")
	events, err := store.EventsOfPerson(context.Background(), "ana souza")
	uids := []string{events[0].UID, events[len(events)-1].UID}
	if !slices.Equal(names, []string{"ana prado", "ana souza"}) || err != nil || len(events) != 2 || !slices.Contains(uids, "souza") || !slices.Contains(uids, "caio-mention") {
		t.Fatalf("expected both Anas, and Ana Souza's commit and the @Ana mention, got %v and %v (err %v)", names, uids, err)
	}
}
