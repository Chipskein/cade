package storage

import (
	"context"

	"github.com/chipskein/cade/internal/event"
)

// EventCursor marks where a walk over the stored events stopped; the zero
// cursor starts at the beginning.
type EventCursor int64

// EventPage is one page of a walk: its events, where the next page starts,
// and whether this was the last.
type EventPage struct {
	Events []event.Event
	Next   EventCursor
	Done   bool
}

// EventPager walks the events satisfying a filter a page at a time, in the
// order they were stored, so exporting the whole history never holds it in
// memory (#85).
type EventPager interface {
	// EventsAfter returns up to limit events after cursor.
	//
	//	page, err := store.EventsAfter(ctx, storage.EventFilter{To: now}, 0, 500)
	EventsAfter(ctx context.Context, filter EventFilter, after EventCursor, limit int) (EventPage, error)
}
