package timeline

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// Lister reads the unified, chronological timeline from a store. It never
// branches on the source, so new sources appear automatically (CA11).
type Lister struct {
	store storage.EventStore
}

// NewLister returns a Lister over store.
//
//	events, err := timeline.NewLister(store).List(ctx, days, "")
func NewLister(store storage.EventStore) Lister {
	return Lister{store: store}
}

// List returns the events in days, oldest first; source narrows to one
// source when non-empty.
func (l Lister) List(ctx context.Context, days DayRange, source event.Source) ([]event.Event, error) {
	events, err := l.store.EventsBetween(ctx, days.Start(), days.End())
	if err != nil {
		return nil, fmt.Errorf("load timeline for %s: %w", days, err)
	}
	return filterBySource(events, source), nil
}

func filterBySource(events []event.Event, source event.Source) []event.Event {
	if source == "" {
		return events
	}
	var kept []event.Event
	for _, ev := range events {
		if ev.Source == source {
			kept = append(kept, ev)
		}
	}
	return kept
}
