package timeline

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// A file is one event at its latest version; the earlier edit of the day
// must still show up, and the latest must not appear twice.
func TestListShowsEarlierFileVersions(t *testing.T) {
	morning, afternoon := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	store := testfakes.NewFakeEventStore()
	current := event.Event{UID: "f", Source: event.SourceFile, Timestamp: afternoon, Content: "a.md\ntexto",
		Metadata: event.File{Path: "/notas/a.md", ModifiedAt: afternoon}.Metadata()}
	commit := event.Event{UID: "c", Source: event.SourceGit, Timestamp: morning.Add(time.Hour), Content: "commit"}
	store.Events = []event.Event{commit, current}
	store.Modifications = []storage.FileModification{{Path: "/notas/a.md", ModifiedAt: morning, Size: 3}, {Path: "/notas/a.md", ModifiedAt: afternoon, Size: 5}}
	days, _ := ParseDayRange("2026-09-25", "", afternoon)
	events, err := NewLister(store).List(context.Background(), days, "")
	if err != nil || len(events) != 3 {
		t.Fatalf("expected the earlier version, the commit and the file, got %d (err %v)", len(events), err)
	}
	if !events[0].Timestamp.Equal(morning) || events[0].Content != "a.md" || events[0].File().Path != "/notas/a.md" || events[2].UID != "f" {
		t.Fatalf("expected the earlier version first, path only, and the current file last, got %+v", events)
	}
	files, _ := NewLister(store).List(context.Background(), days, event.SourceFile)
	if len(files) != 2 {
		t.Fatalf("expected both versions when filtering files, got %d", len(files))
	}
}
