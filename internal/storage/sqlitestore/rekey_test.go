package sqlitestore

import (
	"context"
	"os"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestRekeyEventsKeepsContentAndChunks(t *testing.T) {
	store, ctx := openTestStore(t), context.Background()
	mustSave(t, store, sampleEvent("old-a", event.SourceTeams, 0), []float32{1, 0})
	mustSave(t, store, sampleEvent("old-b", event.SourceTeams, 1), []float32{0, 1})
	report, err := store.RekeyEvents(ctx, map[string]string{"old-a": "new-a", "missing": "new-x"})
	if err != nil || report.Rekeyed != 1 || report.Conflicts != 0 {
		t.Fatalf("expected one event rekeyed, got %+v (err %v)", report, err)
	}
	stored, found, _ := store.StoredEvent(ctx, "new-a")
	_, oldFound, _ := store.StoredEvent(ctx, "old-a")
	if !found || oldFound || stored.Content != "content old-a" {
		t.Fatalf("expected the event under its new UID with its content, got %+v (old still there: %v)", stored, oldFound)
	}
	if chunks, _ := store.ChunksFor(ctx, []string{"new-a"}); len(chunks["new-a"]) == 0 {
		t.Fatal("expected the chunks to follow the event")
	}
}

func TestRekeyEventsBacksUpFirst(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("old-a", event.SourceTeams, 0), nil)
	report, err := store.RekeyEvents(context.Background(), map[string]string{"old-a": "new-a"})
	if err != nil || report.BackupPath == "" {
		t.Fatalf("expected a backup path, got %+v (err %v)", report, err)
	}
	info, err := os.Stat(report.BackupPath)
	if err != nil || info.Mode().Perm() != privateFileMode {
		t.Fatalf("expected an owner-only backup, got %v (err %v)", info, err)
	}
}

// A message ingested again under its new UID before the rekey is the same
// message: the rekey leaves both as they are and counts the conflict.
func TestRekeyEventsCountsConflicts(t *testing.T) {
	store, ctx := openTestStore(t), context.Background()
	mustSave(t, store, sampleEvent("old-a", event.SourceTeams, 0), nil)
	mustSave(t, store, sampleEvent("new-a", event.SourceTeams, 0), nil)
	report, err := store.RekeyEvents(ctx, map[string]string{"old-a": "new-a"})
	if err != nil || report.Rekeyed != 0 || report.Conflicts != 1 {
		t.Fatalf("expected one conflict, got %+v (err %v)", report, err)
	}
}

func TestRekeyEventsKeepsForgottenEventsForgotten(t *testing.T) {
	store, ctx := openTestStore(t), context.Background()
	mustSave(t, store, sampleEvent("old-a", event.SourceTeams, 0), nil)
	if _, err := store.DeleteEvent(ctx, "old-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RekeyEvents(ctx, map[string]string{"old-a": "new-a"}); err != nil {
		t.Fatal(err)
	}
	if forgotten, err := store.IsForgotten(ctx, "new-a"); err != nil || !forgotten {
		t.Fatalf("expected the new UID forgotten too, got %v (err %v)", forgotten, err)
	}
}

func TestRekeyEventsWithNothingToChange(t *testing.T) {
	if report, err := openTestStore(t).RekeyEvents(context.Background(), nil); err != nil || report.BackupPath != "" {
		t.Fatalf("expected no backup for an empty change set, got %+v (err %v)", report, err)
	}
}
