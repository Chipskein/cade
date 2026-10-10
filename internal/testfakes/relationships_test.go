package testfakes

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
)

func commitStore() (*FakeEventStore, int64) {
	store := NewFakeEventStore()
	store.Events = []event.Event{
		{UID: "rui", Source: event.SourceGit, Metadata: event.Commit{Repository: "/src/api", Author: "Rui Costa", Files: []string{"erp/retry.go"}}.Metadata()},
		{UID: "carla", Source: event.SourceGit, Metadata: event.Commit{Repository: "/src/api", Author: "Carla Dias", Files: []string{"erp/retry.go"}}.Metadata()},
	}
	links, _ := store.EntitiesOfEvent(context.Background(), "rui")
	return store, links[0].ID
}

func TestFakeRelationsMatchTheCommits(t *testing.T) {
	store, rui := commitStore()
	from, err := store.RelationsFrom(context.Background(), rui, entity.RelationChanged)
	if err != nil || len(from) != 1 || from[0].To.Key != "/src/api/erp/retry.go" || from[0].SourceEventUID != "rui" {
		t.Fatalf("expected Rui changing retry.go, got %+v (err %v)", from, err)
	}
	to, err := store.RelationsTo(context.Background(), from[0].To.ID, "")
	if err != nil || len(to) != 2 {
		t.Fatalf("expected both authors changing retry.go, got %+v (err %v)", to, err)
	}
}

func TestFakeReachableFindsTheOtherAuthorTwoHopsAway(t *testing.T) {
	store, rui := commitStore()
	reached, err := store.Reachable(context.Background(), rui, 2)
	if err != nil || len(reached) != 2 || reached[0].Hops != 1 || reached[1].Key != "carla dias" || reached[1].Hops != 2 {
		t.Fatalf("expected retry.go then Carla, got %+v (err %v)", reached, err)
	}
}
