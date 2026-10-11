package sqlitestore

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// walk reads every page of filter, limit events at a time.
func walk(t *testing.T, pager storage.EventPager, filter storage.EventFilter, limit int) ([]string, int) {
	t.Helper()
	var uids []string
	pages := 0
	for cursor := storage.EventCursor(0); ; pages++ {
		page, err := pager.EventsAfter(context.Background(), filter, cursor, limit)
		testcheck.NoError(t, err)
		uids, cursor = append(uids, uidsOf(page.Events)...), page.Next
		if page.Done {
			return uids, pages + 1
		}
	}
}

func TestEventsAfterWalksEveryEventOnceInStoredOrder(t *testing.T) {
	store := openTestStore(t)
	for i, uid := range []string{"c", "a", "e", "b", "d"} {
		mustSave(t, store, sampleEvent(uid, []event.Source{event.SourceGit, event.SourceFile}[i%2], time.Duration(i)*time.Hour), nil)
	}
	all := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0)}
	if uids, pages := walk(t, store, all, 2); !slices.Equal(uids, []string{"c", "a", "e", "b", "d"}) || pages != 3 {
		t.Fatalf("expected every event in stored order over 3 pages, got %v in %d", uids, pages)
	}
	files := all
	files.Source = event.SourceFile
	if uids, _ := walk(t, store, files, 10); !slices.Equal(uids, []string{"a", "b"}) {
		t.Fatalf("expected only the files, got %v", uids)
	}
}

func TestRelationsStatedByAreTheEventsOwn(t *testing.T) {
	store := openTestStore(t)
	commit := func(uid, author string) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime, Content: "commit " + uid,
			Metadata: event.Commit{Repository: "/src/api", Author: author, Hash: uid, Files: []string{"erp/retry.go"}}.Metadata()}
	}
	mustSave(t, store, commit("rui", "Rui Costa"), nil)
	mustSave(t, store, commit("carla", "Carla Dias"), nil)
	relations, err := store.RelationsStatedBy(context.Background(), "rui")
	if err != nil || len(relations) != 1 || relations[0].From.Name != "rui costa" || relations[0].Relation != entity.RelationChanged || relations[0].SourceEventUID != "rui" {
		t.Fatalf("expected only Rui's change, got %+v (err %v)", relations, err)
	}
}
