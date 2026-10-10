package testfakes

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func TestFakeEntitiesMatchTheEventsThatReferToThem(t *testing.T) {
	store := NewFakeEventStore()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	store.Events = []event.Event{
		{UID: "c1", Source: event.SourceGit, Timestamp: at.Add(time.Hour), Metadata: event.Commit{Author: "Ana Souza"}.Metadata()},
		{UID: "m1", Source: event.SourceTeams, Timestamp: at, Metadata: event.Message{Sender: "Ana Souza"}.Metadata()},
	}
	links, err := store.EntitiesOfEvent(context.Background(), "c1")
	if err != nil || len(links) != 1 || links[0].Name != "ana souza" {
		t.Fatalf("expected Ana Souza as the author of c1, got %+v (%v)", links, err)
	}
	uids, err := store.EventsOfEntity(context.Background(), links[0].ID)
	if err != nil || !slices.Equal(uids, []string{"m1", "c1"}) {
		t.Fatalf("expected m1 then c1, got %q (%v)", uids, err)
	}
	again, _ := store.EntitiesOfEvent(context.Background(), "m1")
	if len(again) != 1 || again[0].ID != links[0].ID {
		t.Fatalf("expected the same id from both events, got %+v and %+v", links, again)
	}
}

func TestFakeEventsOfUnknownEntityAreEmpty(t *testing.T) {
	store := NewFakeEventStore()
	store.Events = []event.Event{{UID: "m1", Source: event.SourceTeams, Metadata: event.Message{Sender: "Ana Souza"}.Metadata()}}
	if uids, err := store.EventsOfEntity(context.Background(), 0); err != nil || len(uids) != 0 {
		t.Fatalf("expected no events for an id never given, got %q (%v)", uids, err)
	}
}
