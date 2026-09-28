package listing

import "github.com/chipskein/cade/internal/event"

// InFolder keeps the file events under folder; an empty folder keeps all.
// It is the in-memory reference for storage.EventFilter.Folder.
//
//	kept := listing.InFolder(events, "/home/ana/Documents")
func InFolder(events []event.Event, folder string) []event.Event {
	if folder == "" {
		return events
	}
	var kept []event.Event
	for _, ev := range events {
		if ev.Source == event.SourceFile && ev.File().IsUnder(folder) {
			kept = append(kept, ev)
		}
	}
	return kept
}
