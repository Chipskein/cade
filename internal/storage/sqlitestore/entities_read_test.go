package sqlitestore

import (
	"context"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestEntitiesOfEventAndBackToItsEvents(t *testing.T) {
	store := storeAcrossSources(t)
	links, err := store.EntitiesOfEvent(context.Background(), "m1")
	testcheck.NoError(t, err)
	if len(links) != 1 || links[0].Type != entity.TypePerson || links[0].Key != "ana souza" || links[0].Name != "ana souza" || links[0].Role != entity.RoleSender {
		t.Fatalf("expected Ana Souza as the sender of m1, got %+v", links)
	}
	uids, err := store.EventsOfEntity(context.Background(), links[0].ID)
	testcheck.NoError(t, err)
	if want := []string{"m1", "c2"}; !slices.Equal(uids, want) {
		t.Fatalf("expected her message and commit oldest first %q, got %q", want, uids)
	}
}

func TestEntityReadsOfUnknownAreEmpty(t *testing.T) {
	store := storeAcrossSources(t)
	links, err := store.EntitiesOfEvent(context.Background(), "missing")
	testcheck.NoError(t, err)
	uids, err := store.EventsOfEntity(context.Background(), -1)
	testcheck.NoError(t, err)
	if len(links) != 0 || len(uids) != 0 {
		t.Fatalf("expected nothing for unknown ids, got %+v and %q", links, uids)
	}
}
