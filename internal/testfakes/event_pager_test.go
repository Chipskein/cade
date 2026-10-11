package testfakes

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

func TestFakeEventsAfterPagesAndRelationsStatedBy(t *testing.T) {
	store := NewFakeEventStore()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i, uid := range []string{"a", "b", "c"} {
		store.Events = append(store.Events, event.Event{UID: uid, Source: event.SourceGit, Timestamp: at.Add(time.Duration(i) * time.Hour),
			Metadata: event.Commit{Repository: "/src", Author: "Rui Costa", Files: []string{uid + ".go"}}.Metadata()})
	}
	filter := storage.EventFilter{To: at.AddDate(0, 0, 1)}
	first, _ := store.EventsAfter(context.Background(), filter, 0, 2)
	second, _ := store.EventsAfter(context.Background(), filter, first.Next, 2)
	if len(first.Events) != 2 || first.Done || !second.Done || second.Events[0].UID != "c" {
		t.Fatalf("expected two pages, got %+v and %+v", first, second)
	}
	relations, err := store.RelationsStatedBy(context.Background(), "b")
	if err != nil || len(relations) != 1 || !slices.Contains([]string{"/src/b.go"}, relations[0].To.Key) {
		t.Fatalf("expected b's change of b.go, got %+v (err %v)", relations, err)
	}
}
