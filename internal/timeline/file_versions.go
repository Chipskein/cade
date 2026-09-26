package timeline

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// A file is one event dated at its latest version; its earlier edits live
// only in the file history. withFileVersions puts each of those edits back
// in the timeline, so "o que editei ontem?" still sees them.
func (l Lister) withFileVersions(ctx context.Context, days DayRange, events []event.Event) ([]event.Event, error) {
	modifications, err := l.store.FileModificationsBetween(ctx, days.Start(), days.End())
	if err != nil {
		return nil, fmt.Errorf("load file edits for %s: %w", days, err)
	}
	stored := map[string]bool{}
	for _, ev := range events {
		if ev.Source == event.SourceFile {
			stored[versionKey(ev.File().Path, ev.Timestamp)] = true
		}
	}
	for _, modification := range modifications {
		if !stored[versionKey(modification.Path, modification.ModifiedAt)] {
			events = append(events, versionEvent(modification))
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp.Before(events[j].Timestamp) })
	return events, nil
}

func versionKey(path string, modifiedAt time.Time) string {
	return fmt.Sprintf("%s@%d", path, modifiedAt.UnixMilli())
}

// versionEvent is a timeline entry for an earlier version: the path only,
// since the text of past versions is not kept.
func versionEvent(modification storage.FileModification) event.Event {
	file := event.File{Path: modification.Path, Size: modification.Size, ModifiedAt: modification.ModifiedAt}
	return event.Event{UID: "file-version:" + versionKey(modification.Path, modification.ModifiedAt), Source: event.SourceFile,
		Timestamp: modification.ModifiedAt, Content: filepath.Base(modification.Path), Metadata: file.Metadata()}
}
