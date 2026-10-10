package timeline

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

func aroundStore() *testfakes.FakeEventStore {
	commit := time.Date(2026, 9, 22, 15, 20, 0, 0, saoPaulo)
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{
		{UID: "early", Source: event.SourceBrowser, Timestamp: commit.Add(-3 * time.Hour)},
		{UID: "page", Source: event.SourceBrowser, Timestamp: commit.Add(-15 * time.Minute)},
		{UID: "commit", Source: event.SourceGit, Timestamp: commit},
		{UID: "edge", Source: event.SourceTeams, Timestamp: commit.Add(2 * time.Hour)},
		{UID: "late", Source: event.SourceTeams, Timestamp: commit.Add(2*time.Hour + time.Minute)},
	}
	return store
}

func uidsOf(events []event.Event) []string {
	uids := make([]string, len(events))
	for i, ev := range events {
		uids[i] = ev.UID
	}
	return uids
}

func TestAroundKeepsTheWindowBothWays(t *testing.T) {
	near, found, err := NewLister(aroundStore()).Around(context.Background(), "commit", DefaultAroundWindow, "")
	if err != nil || !found || near.Anchor.UID != "commit" || near.Window != DefaultAroundWindow {
		t.Fatalf("expected the commit found, got %+v (found %v, err %v)", near.Anchor, found, err)
	}
	if got := uidsOf(near.Events); len(got) != 3 || got[0] != "page" || got[1] != "commit" || got[2] != "edge" {
		t.Fatalf("expected page, commit and the event exactly 2h later, got %q", got)
	}
}

func TestAroundSourceNarrowsNeighborsButKeepsTheAnchor(t *testing.T) {
	near, _, err := NewLister(aroundStore()).Around(context.Background(), "commit", DefaultAroundWindow, event.SourceBrowser)
	if got := uidsOf(near.Events); err != nil || len(got) != 2 || got[0] != "page" || got[1] != "commit" {
		t.Fatalf("expected the page and the anchor, got %q (err %v)", got, err)
	}
}

func TestAroundUnknownUIDIsNotFound(t *testing.T) {
	if _, found, err := NewLister(aroundStore()).Around(context.Background(), "missing", time.Hour, ""); found || err != nil {
		t.Fatalf("expected not found without error, got found=%v err=%v", found, err)
	}
}

func TestAroundRejectsANonPositiveWindow(t *testing.T) {
	if _, _, err := NewLister(aroundStore()).Around(context.Background(), "commit", 0, ""); err == nil {
		t.Fatal("expected an error for a zero window")
	}
}

func TestAroundPutsTheAnchorBackInTimeOrder(t *testing.T) {
	near, _, err := NewLister(aroundStore()).Around(context.Background(), "commit", DefaultAroundWindow, event.SourceTeams)
	if got := uidsOf(near.Events); err != nil || len(got) != 2 || got[0] != "commit" || got[1] != "edge" {
		t.Fatalf("expected the anchor before the later message, got %q (err %v)", got, err)
	}
}
