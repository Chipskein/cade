package timeline

import (
	"context"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// DefaultAroundWindow is how far `timeline --around` looks each way: what
// was open or said just before a commit or a message is usually within a
// sitting, and two hours covers one without pulling in the whole day.
const DefaultAroundWindow = 2 * time.Hour

// Neighborhood is an event and what happened within a window of it.
type Neighborhood struct {
	Anchor event.Event
	Window time.Duration
	// Events are oldest first and include Anchor.
	Events []event.Event
}

// Around returns the stored event uid with the events window before and
// after it; source narrows the neighbors, never the anchor. found is false
// for an unknown uid.
//
//	near, found, err := timeline.NewLister(store).Around(ctx, uid, timeline.DefaultAroundWindow, "")
func (l Lister) Around(ctx context.Context, uid string, window time.Duration, source event.Source) (Neighborhood, bool, error) {
	if window <= 0 {
		return Neighborhood{}, false, fmt.Errorf("window %s, expected a positive duration such as 2h", window)
	}
	anchor, found, err := l.store.StoredEvent(ctx, uid)
	if err != nil || !found {
		return Neighborhood{}, false, err
	}
	events, err := l.store.EventsBetween(ctx, anchor.Timestamp.Add(-window), anchor.Timestamp.Add(window+time.Nanosecond))
	if err != nil {
		return Neighborhood{}, false, fmt.Errorf("load events around %q: %w", uid, err)
	}
	return Neighborhood{Anchor: anchor, Window: window, Events: keepAnchor(filterBySource(events, source), anchor)}, true, nil
}

// keepAnchor puts the anchor back when the source filter removed it.
func keepAnchor(events []event.Event, anchor event.Event) []event.Event {
	for _, ev := range events {
		if ev.UID == anchor.UID {
			return events
		}
	}
	for i, ev := range events {
		if ev.Timestamp.After(anchor.Timestamp) {
			return append(events[:i:i], append([]event.Event{anchor}, events[i:]...)...)
		}
	}
	return append(events, anchor)
}
