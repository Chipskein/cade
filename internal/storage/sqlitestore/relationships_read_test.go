package sqlitestore

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func entityIDOf(t *testing.T, store *Store, kind entity.Type, key string) int64 {
	t.Helper()
	var id int64
	testcheck.NoError(t, store.db.QueryRow(`SELECT id FROM entities WHERE type = ? AND entity_key = ?`, string(kind), key).Scan(&id))
	return id
}

func reachedKeys(reached []storage.ReachedEntity) map[string]int {
	keys := map[string]int{}
	for _, found := range reached {
		keys[found.Key] = found.Hops
	}
	return keys
}

func TestRelationsFromAndToFilterByRelation(t *testing.T) {
	store := storeWithCommits(t)
	carla := entityIDOf(t, store, entity.TypePerson, "carla dias")
	retry := entityIDOf(t, store, entity.TypeFile, "/src/api/erp/retry.go")
	from, err := store.RelationsFrom(context.Background(), carla, entity.RelationChanged)
	testcheck.NoError(t, err)
	to, err := store.RelationsTo(context.Background(), retry, "")
	testcheck.NoError(t, err)
	if len(from) != 2 || from[0].SourceEventUID != "carla" || from[0].Confidence != nil || from[0].From.Name != "carla dias" {
		t.Fatalf("expected Carla's two stated relations, got %+v", from)
	}
	if len(to) != 2 || to[0].From.Key != "rui costa" || to[1].From.Key != "carla dias" {
		t.Fatalf("expected Rui then Carla changing retry.go, got %+v", to)
	}
	if other, err := store.RelationsFrom(context.Background(), carla, entity.Relation("mentioned")); err != nil || len(other) != 0 {
		t.Fatalf("expected no relation of another type, got %+v (err %v)", other, err)
	}
}

func TestReachableWalksBothWaysUpToTheHops(t *testing.T) {
	store := storeWithCommits(t)
	rui := entityIDOf(t, store, entity.TypePerson, "rui costa")
	one, err := store.Reachable(context.Background(), rui, 1)
	testcheck.NoError(t, err)
	if keys := reachedKeys(one); len(keys) != 1 || keys["/src/api/erp/retry.go"] != 1 {
		t.Fatalf("expected only retry.go one hop away, got %+v", one)
	}
	three, err := store.Reachable(context.Background(), rui, 9)
	testcheck.NoError(t, err)
	keys := reachedKeys(three)
	if len(keys) != 3 || keys["carla dias"] != 2 || keys["/src/api/erp/backoff.go"] != 3 || three[0].Hops != 1 {
		t.Fatalf("expected Carla at 2 and backoff.go at 3, nearest first, got %+v", three)
	}
}
