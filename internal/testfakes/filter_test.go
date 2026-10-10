package testfakes

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

func TestFakeEventsMatchingAppliesTheGraphFilter(t *testing.T) {
	store := NewFakeEventStore()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	commit := func(uid, author string) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: at, Content: "commit " + uid,
			Metadata: event.Commit{Repository: "/src", Author: author, Files: []string{"erp/retry.go"}}.Metadata()}
	}
	photo := event.Message{Sender: "Pedro Alves", Kind: event.KindOther}.Metadata()
	photo[event.AttachmentKey] = "IMG-1.jpg"
	store.Events = []event.Event{commit("rui", "Rui Costa"), commit("carla", "Carla Dias"),
		{UID: "photo", Source: event.SourceWhatsApp, Timestamp: at, Content: "Pedro Alves: ", Metadata: photo}}
	file := fileOf(t, store, "rui")
	cases := map[string]storage.GraphFilter{"photo": {Modality: entity.TypeImage}, "rui carla": {EntityIDs: []int64{file}},
		"carla": {EntityIDs: []int64{file}, Appearing: []string{"carla"}}}
	for want, graph := range cases {
		events, err := store.EventsMatching(context.Background(), storage.EventFilter{To: at.Add(time.Hour), Graph: graph})
		if got := uidsOf(events); err != nil || !slices.Equal(got, strings.Fields(want)) {
			t.Errorf("%+v: expected %s, got %v (%v)", graph, want, got, err)
		}
	}
}

func fileOf(t *testing.T, store *FakeEventStore, uid string) int64 {
	t.Helper()
	links, _ := store.EntitiesOfEvent(context.Background(), uid)
	for _, link := range links {
		if link.Type == entity.TypeFile {
			return link.ID
		}
	}
	t.Fatalf("expected %s to link a file", uid)
	return 0
}

func uidsOf(events []event.Event) []string {
	var uids []string
	for _, ev := range events {
		uids = append(uids, ev.UID)
	}
	return uids
}
