package testfakes

import (
	"context"

	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EventPager = (*FakeEventStore)(nil)

// EventsAfter pages through what EventsMatching returns; the cursor is a
// position in that list, not a row id.
func (f *FakeEventStore) EventsAfter(ctx context.Context, filter storage.EventFilter, after storage.EventCursor, limit int) (storage.EventPage, error) {
	matching, err := f.EventsMatching(ctx, filter)
	if err != nil {
		return storage.EventPage{}, err
	}
	start := min(int(after), len(matching))
	end := min(start+limit, len(matching))
	return storage.EventPage{Events: matching[start:end], Next: storage.EventCursor(end), Done: end == len(matching)}, nil
}
