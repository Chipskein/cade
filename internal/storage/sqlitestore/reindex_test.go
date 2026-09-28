package sqlitestore

import (
	"context"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestEmbeddingModelIsRecorded(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	empty, _ := store.EmbeddingModel(ctx)
	testcheck.NoError(t, store.RecordEmbeddingModel(ctx, "a.gguf"))
	testcheck.NoError(t, store.RecordEmbeddingModel(ctx, "b.gguf"))
	if model, _ := store.EmbeddingModel(ctx); empty != "" || model != "b.gguf" {
		t.Fatalf("expected no model then the last recorded, got %q and %q", empty, model)
	}
}

func TestThresholdCalibrationRoundTrips(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	calibration := storage.ThresholdCalibration{Model: "nomic.gguf", MaxDistance: 0.72, MaxBestDistance: 0.61}
	testcheck.NoError(t, store.RecordThresholdCalibration(ctx, calibration))
	got, err := store.ThresholdCalibration(ctx)
	if err != nil || got != calibration {
		t.Fatalf("expected %+v back, got %+v: %v", calibration, got, err)
	}
}

func TestThresholdCalibrationIsZeroBeforeRecorded(t *testing.T) {
	got, err := openTestStore(t).ThresholdCalibration(context.Background())
	if err != nil || got != (storage.ThresholdCalibration{}) {
		t.Fatalf("expected no calibration, got %+v: %v", got, err)
	}
}

// A new model may have another dimension; the vector table must be rebuilt.
func TestReindexAcceptsANewDimension(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	commit, empty := sampleEvent("a", event.SourceGit, 0), sampleEvent("b", event.SourceFile, 0)
	empty.Content = ""
	mustSave(t, store, commit, []float32{1, 0})
	mustSave(t, store, empty, nil)
	if err := store.StartReindex(ctx, "novo.gguf"); err != nil {
		t.Fatal(err)
	}
	missing, _ := store.EventsWithoutEmbedding(ctx, 10)
	count, _ := store.CountEventsWithoutEmbedding(ctx)
	if len(missing) != 1 || missing[0].UID != "a" || count != 1 {
		t.Fatalf("expected only the event with text missing, got %+v (count %d)", missing, count)
	}
	if err := store.SaveEmbeddings(ctx, []storage.EventEmbedding{{Event: commit, Chunks: whole(commit, []float32{0, 0, 1})}}); err != nil {
		t.Fatal(err)
	}
	vectors := firstVectors(t, store, "a")
	remaining, _ := store.CountEventsWithoutEmbedding(ctx)
	if len(vectors["a"]) != 3 || remaining != 0 {
		t.Fatalf("expected the 3-dim vector stored and nothing left, got %v (%d left)", vectors, remaining)
	}
}

func TestReindexPendingUntilFinished(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	testcheck.NoError(t, store.StartReindex(ctx, "novo.gguf"))
	pending, _ := store.ReindexPending(ctx)
	testcheck.NoError(t, store.FinishReindex(ctx))
	finished, _ := store.ReindexPending(ctx)
	if model, _ := store.EmbeddingModel(ctx); !pending || finished || model != "novo.gguf" {
		t.Fatalf("expected pending then finished with the model recorded, got %v %v %q", pending, finished, model)
	}
}

func TestSaveEmbeddingsRejectsUnknownEvent(t *testing.T) {
	store := openTestStore(t)
	err := store.SaveEmbeddings(context.Background(), []storage.EventEmbedding{{Event: event.Event{UID: "zz"}, Chunks: whole(event.Event{UID: "zz", Content: "x"}, []float32{1})}})
	if err == nil || !strings.Contains(err.Error(), `"zz"`) {
		t.Fatalf("expected an error naming the uid, got %v", err)
	}
}
