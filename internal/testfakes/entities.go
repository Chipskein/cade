package testfakes

import (
	"context"
	"slices"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EntityIndex = (*FakeEventStore)(nil)

// entityIdentity is what makes two refs the same entity.
type entityIdentity struct {
	Type entity.Type
	Key  string
}

// EntitiesOfEvent derives the event's entities with entity.RefsOf, as the
// real store does when it saves the event.
func (f *FakeEventStore) EntitiesOfEvent(_ context.Context, uid string) ([]storage.EntityLink, error) {
	index := f.indexOf(uid)
	if index < 0 {
		return nil, f.FailWith
	}
	var links []storage.EntityLink
	for _, ref := range entity.RefsOf(f.Events[index]) {
		links = append(links, storage.EntityLink{Entity: f.storedEntity(ref), Role: ref.Role})
	}
	return links, f.FailWith
}

// EventsOfEntity lists the events referring to the entity, oldest first.
func (f *FakeEventStore) EventsOfEntity(_ context.Context, entityID int64) ([]string, error) {
	if entityID <= 0 {
		return nil, f.FailWith
	}
	events := slices.Clone(f.Events)
	slices.SortStableFunc(events, func(a, b event.Event) int { return a.Timestamp.Compare(b.Timestamp) })
	var uids []string
	for _, ev := range events {
		if slices.ContainsFunc(entity.RefsOf(ev), func(ref entity.Ref) bool { return f.knownID(ref) == entityID }) {
			uids = append(uids, ev.UID)
		}
	}
	return uids, f.FailWith
}

func (f *FakeEventStore) entityID(identity entityIdentity) int64 {
	if f.entityIDs == nil {
		f.entityIDs = map[entityIdentity]int64{}
	}
	if _, found := f.entityIDs[identity]; !found {
		f.entityIDs[identity] = int64(len(f.entityIDs) + 1)
	}
	return f.entityIDs[identity]
}

// knownID is the id already given to ref's entity, 0 when none was.
func (f *FakeEventStore) knownID(ref entity.Ref) int64 {
	return f.entityIDs[entityIdentity{Type: ref.Type, Key: ref.Key}]
}

// firstSpelling is the name of the earliest stored event naming the
// entity; unlike the real store, it forgets a spelling with its event.
func (f *FakeEventStore) firstSpelling(identity entityIdentity) string {
	for _, ev := range f.Events {
		for _, ref := range entity.RefsOf(ev) {
			if ref.Type == identity.Type && ref.Key == identity.Key {
				return ref.Name
			}
		}
	}
	return ""
}
