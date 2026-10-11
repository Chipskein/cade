package rag

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// A question asking for others ("quem mais mexeu no arquivo que o Rui
// alterou?") starts at the people it names and follows their
// relationships: Rui changed erp/retry.go, so every event on that file,
// his and Carla's, is a candidate. The people stop being a filter; the
// topic and the vectors rank what the graph reached (#24).

// relatedHops reaches what the people touched, not who else touched it:
// those are found through the events of the reached entities.
const relatedHops = 1

// widenByRelations swaps filter's people for the entities they relate to;
// filter is unchanged when none of them relates to anything.
func (a *Answerer) widenByRelations(ctx context.Context, filter storage.EventFilter) (storage.EventFilter, error) {
	seeds, err := a.personEntities(ctx, filter)
	if err != nil || len(seeds) == 0 {
		return filter, err
	}
	reached, err := a.reachedEntities(ctx, seeds)
	if err != nil || len(reached) == 0 {
		return filter, err
	}
	filter.People, filter.Graph.EntityIDs = nil, reached
	return filter, nil
}

// personEntities finds each named person's entity through their events;
// every event of a person links the same entity, so it stops once each
// person has one.
func (a *Answerer) personEntities(ctx context.Context, filter storage.EventFilter) ([]int64, error) {
	events, err := a.store.EventsMatching(ctx, filter)
	if err != nil {
		return nil, err
	}
	found := map[string]int64{}
	for _, ev := range events {
		links, err := a.store.EntitiesOfEvent(ctx, ev.UID)
		if err != nil {
			return nil, err
		}
		addNamedPeople(found, links, filter.People)
		if len(found) == len(filter.People) {
			break
		}
	}
	return slices.Sorted(maps.Values(found)), nil
}

// addNamedPeople records, by the person's key, each Person link the
// people name, as the people filter matches names: whole words.
func addNamedPeople(found map[string]int64, links []storage.EntityLink, people []listing.PersonMatcher) {
	for _, person := range people {
		named := func(link storage.EntityLink) bool {
			return link.Type == entity.TypePerson && strings.Contains(" "+link.Key+" ", " "+person.Key()+" ")
		}
		if index := slices.IndexFunc(links, named); index >= 0 {
			found[person.Key()] = links[index].ID
		}
	}
}

// reachedEntities are what the seeds relate to, people left out.
func (a *Answerer) reachedEntities(ctx context.Context, seeds []int64) ([]int64, error) {
	var reached []int64
	for _, seed := range seeds {
		entities, err := a.store.Reachable(ctx, seed, relatedHops)
		if err != nil {
			return nil, err
		}
		reached = append(reached, idsOfThings(entities)...)
	}
	return reached, nil
}

// idsOfThings are the ids of the entities that are not people.
func idsOfThings(entities []storage.ReachedEntity) []int64 {
	var ids []int64
	for _, other := range entities {
		if other.Type != entity.TypePerson {
			ids = append(ids, other.ID)
		}
	}
	return ids
}
