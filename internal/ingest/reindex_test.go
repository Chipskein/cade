package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

func storeWithEvents(count int) *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	for i := range count {
		uid := string(rune('a' + i))
		store.Events = append(store.Events, event.Event{UID: uid, Source: event.SourceGit, Timestamp: time.Unix(int64(i), 0), Content: "commit " + uid})
	}
	store.Events = append(store.Events, event.Event{UID: "sem-texto", Source: event.SourceFile})
	return store
}

func TestReindexEmbedsEveryEventWithText(t *testing.T) {
	store, embedder := storeWithEvents(3), &testfakes.FakeEmbedder{}
	var progress [][2]int
	done, err := newTestPipeline(store, embedder).Reindex(context.Background(), store, "modelo-b", func(d, total int) { progress = append(progress, [2]int{d, total}) })
	if err != nil || done != 3 || len(embedder.Inputs) != 3 || store.Pending || store.EmbeddingModelName != "modelo-b" {
		t.Fatalf("expected 3 embedded and the rebuild finished, got %d (err %v, pending %v, model %q)", done, err, store.Pending, store.EmbeddingModelName)
	}
	if len(progress) != 1 || progress[0] != [2]int{3, 3} || embedder.Inputs[0] != "doc: commit a" {
		t.Fatalf("unexpected progress %v or input %q", progress, embedder.Inputs[0])
	}
}

// An interrupted rebuild with the same model continues where it stopped.
func TestReindexResumesPendingRebuild(t *testing.T) {
	store, embedder := storeWithEvents(3), &testfakes.FakeEmbedder{}
	store.EmbeddingModelName, store.Pending = "modelo-b", true
	store.Embeddings = map[string][]float32{"a": {1}, "b": {1}}
	done, err := newTestPipeline(store, embedder).Reindex(context.Background(), store, "modelo-b", nil)
	if err != nil || done != 1 || store.ReindexStarted != 0 || len(embedder.Inputs) != 1 {
		t.Fatalf("expected only the missing event embedded, got %d (err %v, restarts %d)", done, err, store.ReindexStarted)
	}
}

func TestReindexStartsOverForAnotherModel(t *testing.T) {
	store := storeWithEvents(2)
	store.EmbeddingModelName, store.Pending = "modelo-a", true
	store.Embeddings = map[string][]float32{"a": {1}}
	done, _ := newTestPipeline(store, &testfakes.FakeEmbedder{}).Reindex(context.Background(), store, "modelo-b", nil)
	if done != 2 || store.ReindexStarted != 1 {
		t.Fatalf("expected a fresh rebuild of 2, got %d with %d restarts", done, store.ReindexStarted)
	}
}

func TestReindexFailureKeepsItPending(t *testing.T) {
	store := storeWithEvents(2)
	embedder := &testfakes.FakeEmbedder{FailWith: errors.New("out of memory")}
	if _, err := newTestPipeline(store, embedder).Reindex(context.Background(), store, "modelo-b", nil); err == nil || !store.Pending {
		t.Fatalf("expected an error and the rebuild still pending, got %v (pending %v)", err, store.Pending)
	}
}
