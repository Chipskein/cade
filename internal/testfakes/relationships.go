package testfakes

import (
	"context"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.RelationIndex = (*FakeEventStore)(nil)

// RelationsFrom derives the relations with entity.RelationsOf, as the real
// store does when it saves the events.
func (f *FakeEventStore) RelationsFrom(_ context.Context, entityID int64, relation entity.Relation) ([]storage.Relationship, error) {
	return f.relationsWhere(func(stored storage.Relationship) bool { return stored.From.ID == entityID }, relation), f.FailWith
}

// RelationsTo lists the relations whose target is entityID.
func (f *FakeEventStore) RelationsTo(_ context.Context, entityID int64, relation entity.Relation) ([]storage.Relationship, error) {
	return f.relationsWhere(func(stored storage.Relationship) bool { return stored.To.ID == entityID }, relation), f.FailWith
}

// RelationsStatedBy lists the derived relations of the event uid.
func (f *FakeEventStore) RelationsStatedBy(_ context.Context, uid string) ([]storage.Relationship, error) {
	return f.relationsWhere(func(stored storage.Relationship) bool { return stored.SourceEventUID == uid }, ""), f.FailWith
}

// Reachable walks the derived relations both ways, breadth first.
func (f *FakeEventStore) Reachable(_ context.Context, entityID int64, hops int) ([]storage.ReachedEntity, error) {
	relations := f.relationsWhere(func(storage.Relationship) bool { return true }, "")
	seen := map[int64]bool{entityID: true}
	frontier := []int64{entityID}
	var reached []storage.ReachedEntity
	for hop := 1; hop <= min(hops, storage.MaxRelationHops) && len(frontier) > 0; hop++ {
		var next []int64
		for _, found := range neighborsOf(relations, frontier) {
			if !seen[found.ID] {
				seen[found.ID] = true
				next = append(next, found.ID)
				reached = append(reached, storage.ReachedEntity{Entity: found, Hops: hop})
			}
		}
		frontier = next
	}
	return reached, f.FailWith
}

// neighborsOf lists the other end of each relation touching frontier.
func neighborsOf(relations []storage.Relationship, frontier []int64) []storage.Entity {
	var found []storage.Entity
	for _, stored := range relations {
		for _, id := range frontier {
			if stored.From.ID == id {
				found = append(found, stored.To)
			}
			if stored.To.ID == id {
				found = append(found, stored.From)
			}
		}
	}
	return found
}

func (f *FakeEventStore) relationsWhere(keep func(storage.Relationship) bool, relation entity.Relation) []storage.Relationship {
	var relations []storage.Relationship
	for _, ev := range f.Events {
		for _, ref := range entity.RelationsOf(ev) {
			stored := storage.Relationship{From: f.storedEntity(ref.From), Relation: ref.Relation, To: f.storedEntity(ref.To), SourceEventUID: ev.UID}
			if keep(stored) && (relation == "" || relation == ref.Relation) {
				relations = append(relations, stored)
			}
		}
	}
	return relations
}

func (f *FakeEventStore) storedEntity(ref entity.Ref) storage.Entity {
	identity := entityIdentity{Type: ref.Type, Key: ref.Key}
	return storage.Entity{ID: f.entityID(identity), Type: ref.Type, Key: ref.Key, Name: f.firstSpelling(identity)}
}
