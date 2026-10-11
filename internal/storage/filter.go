package storage

import (
	"context"
	"slices"

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

// ResolveAppearing keeps the names that appear in some event in scope and
// returns scope narrowed to them, with the names that matched nobody: a
// misread name is dropped, as ResolveCriteria drops an unknown person,
// rather than emptying the result.
//
//	filter, unknown, err := storage.ResolveAppearing(ctx, store, filter, []string{"João"})
func ResolveAppearing(ctx context.Context, store EventStore, scope EventFilter, names []string) (EventFilter, []string, error) {
	var unknown []string
	for _, name := range names {
		probe := scope
		probe.Graph.Appearing = append(slices.Clone(scope.Graph.Appearing), name)
		count, err := store.CountMatching(ctx, probe, 1)
		if err != nil {
			return scope, nil, err
		}
		if count == 0 || len(AppearingWords(name)) == 0 {
			unknown = append(unknown, name)
			continue
		}
		scope = probe
	}
	return scope, unknown, nil
}
