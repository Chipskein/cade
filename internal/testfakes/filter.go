package testfakes

import (
	"context"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// The fake filters with listing.Select, the in-memory reference the real
// store's SQL is tested against.

// CountMatching counts the events EventsMatching would return, up to upTo.
func (f *FakeEventStore) CountMatching(ctx context.Context, filter storage.EventFilter, upTo int) (int, error) {
	events, err := f.EventsMatching(ctx, filter)
	return min(len(events), upTo), err
}

// EventsMatching returns the events in the filter's period and source that
// listing.Select keeps, oldest first.
func (f *FakeEventStore) EventsMatching(ctx context.Context, filter storage.EventFilter) ([]event.Event, error) {
	inPeriod, err := f.EventsBetween(ctx, filter.From, filter.To)
	var scoped []event.Event
	for _, ev := range inPeriod {
		if filter.Source == "" || ev.Source == filter.Source {
			scoped = append(scoped, ev)
		}
	}
	return listing.Select(scoped, filter.Direction, filter.People), err
}

// keepAmong drops the hits whose event does not satisfy among.
func (f *FakeEventStore) keepAmong(hits []storage.ScoredEvent, among *storage.EventFilter) []storage.ScoredEvent {
	if among == nil {
		return hits
	}
	matching, _ := f.EventsMatching(context.Background(), *among)
	allowed := map[string]bool{}
	for _, ev := range matching {
		allowed[ev.UID] = true
	}
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		if allowed[hit.Event.UID] {
			kept = append(kept, hit)
		}
	}
	return kept
}
