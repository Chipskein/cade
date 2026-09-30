package sqlitestore

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func mustBeginBatch(t *testing.T, ctx context.Context, store *Store) storage.EventBatch {
	t.Helper()
	batch, err := store.BeginBatch(ctx)
	testcheck.NoError(t, err)
	t.Cleanup(func() { testcheck.NoError(t, batch.Rollback()) })
	return batch
}

func saveInBatch(t *testing.T, batch storage.EventBatch, ev event.Event) {
	t.Helper()
	if inserted, err := batch.SaveEvent(context.Background(), ev, whole(ev, []float32{1, 0})); err != nil || !inserted {
		t.Fatalf("save %q in batch: inserted=%v err=%v", ev.UID, inserted, err)
	}
}

func assertStored(t *testing.T, store *Store, uid string, want bool) {
	t.Helper()
	if _, found, err := store.StoredEvent(context.Background(), uid); err != nil || found != want {
		t.Fatalf("event %q stored=%v (err %v), expected %v", uid, found, err, want)
	}
}

func TestBatchCommitStoresEveryEvent(t *testing.T) {
	store := openTestStore(t)
	batch := mustBeginBatch(t, context.Background(), store)
	saveInBatch(t, batch, sampleEvent("a", event.SourceGit, 0))
	saveInBatch(t, batch, sampleEvent("b", event.SourceGit, 0))
	testcheck.NoError(t, batch.Commit())
	assertStored(t, store, "a", true)
	assertStored(t, store, "b", true)
}

func TestBatchRollbackKeepsOnlyEarlierCommits(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	batch := mustBeginBatch(t, context.Background(), store)
	saveInBatch(t, batch, sampleEvent("b", event.SourceGit, 0))
	testcheck.NoError(t, batch.Rollback())
	assertStored(t, store, "a", true)
	assertStored(t, store, "b", false)
}

func TestBatchReadsItsOwnWrites(t *testing.T) {
	store := openTestStore(t)
	batch := mustBeginBatch(t, context.Background(), store)
	ev := sampleEvent("a", event.SourceGit, 0)
	saveInBatch(t, batch, ev)
	if _, found, err := batch.StoredEvent(context.Background(), "a"); err != nil || !found {
		t.Fatalf("batch did not see its own event: found=%v err=%v", found, err)
	}
	if chunks, found, err := batch.StoredChunksForContent(context.Background(), ev.Content); err != nil || !found || len(chunks) != 1 {
		t.Fatalf("batch did not see its own chunks: %+v found=%v err=%v", chunks, found, err)
	}
}

func TestBatchUpdateReplacesContent(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	batch := mustBeginBatch(t, context.Background(), store)
	edited := sampleEvent("a", event.SourceGit, 0)
	edited.Content = "edited"
	testcheck.NoError(t, batch.UpdateEvent(context.Background(), edited, whole(edited, []float32{0, 1})))
	testcheck.NoError(t, batch.Commit())
	if stored, _, err := store.StoredEvent(context.Background(), "a"); err != nil || stored.Content != "edited" {
		t.Fatalf("expected content %q, got %q (err %v)", "edited", stored.Content, err)
	}
}

func TestBatchSeesForgottenUID(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	if _, err := store.DeleteEvent(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	batch := mustBeginBatch(t, context.Background(), store)
	if forgotten, err := batch.IsForgotten(context.Background(), "a"); err != nil || !forgotten {
		t.Fatalf("expected %q forgotten, got %v (err %v)", "a", forgotten, err)
	}
}

func TestCancelledBatchLosesOnlyItsEvents(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	ctx, cancel := context.WithCancel(context.Background())
	batch := mustBeginBatch(t, ctx, store)
	saveInBatch(t, batch, sampleEvent("b", event.SourceGit, 0))
	cancel()
	if err := batch.Commit(); err == nil {
		t.Fatal("expected a cancelled batch to refuse its commit")
	}
	testcheck.NoError(t, batch.Rollback())
	assertStored(t, store, "a", true)
	assertStored(t, store, "b", false)
}

func TestBatchRollbackAfterCommitIsSafe(t *testing.T) {
	store := openTestStore(t)
	batch := mustBeginBatch(t, context.Background(), store)
	testcheck.NoError(t, batch.Commit())
	testcheck.NoError(t, batch.Rollback())
}
