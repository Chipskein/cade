package storage

import (
	"context"

	"github.com/chipskein/cade/internal/listing"
)

// ResolveCriteria resolves criteria's people against the stored events in
// scope (scope's period, source and the criteria's direction), and returns
// scope narrowed to them. Each name costs one count query that stops at
// the first match; no event is loaded.
//
//	filter, resolution, err := storage.ResolveCriteria(ctx, store, storage.EventFilter{From: from, To: to}, query.Criteria)
func ResolveCriteria(ctx context.Context, store EventStore, scope EventFilter, criteria listing.Criteria) (EventFilter, listing.Resolution, error) {
	scope.Direction = criteria.Direction
	exists := func(person listing.PersonMatcher) (bool, error) {
		probe := scope
		probe.People = []listing.PersonMatcher{person}
		count, err := store.CountMatching(ctx, probe, 1)
		return count > 0, err
	}
	resolution, err := criteria.Resolve(exists)
	scope.People = resolution.People
	return scope, resolution, err
}
