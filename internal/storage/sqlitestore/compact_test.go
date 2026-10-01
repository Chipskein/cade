package sqlitestore

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// vec0BlockSize is sqlite-vec's default chunk_size: positions per block.
const vec0BlockSize = 1024

// saveVectorEvents stores count events in one batch, alternating git and
// file, each with a distinct direction so the nearest ones are unambiguous.
func saveVectorEvents(t *testing.T, store *Store, count int) {
	t.Helper()
	ctx := context.Background()
	batch, err := store.BeginBatch(ctx)
	testcheck.NoError(t, err)
	t.Cleanup(func() { testcheck.NoError(t, batch.Rollback()) })
	sources := []event.Source{event.SourceGit, event.SourceFile}
	for i := range count {
		ev := sampleEvent(fmt.Sprintf("e%d", i), sources[i%len(sources)], time.Duration(i)*time.Minute)
		angle := float64(i) / float64(count) * math.Pi / 2
		_, err := batch.SaveEvent(ctx, ev, whole(ev, []float32{float32(math.Cos(angle)), float32(math.Sin(angle))}))
		testcheck.NoError(t, err)
	}
	testcheck.NoError(t, batch.Commit())
}

func nearestHits(t *testing.T, store *Store) []storage.ScoredEvent {
	t.Helper()
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0.3}, Limit: 6})
	testcheck.NoError(t, err)
	return hits
}

func blocksFor(vectors int) int {
	return (vectors + vec0BlockSize - 1) / vec0BlockSize * vec0BlockSize
}

func TestCompactVectorsKeepsOnlyTheBlocksTheLiveVectorsNeed(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	saveVectorEvents(t, store, 3*vec0BlockSize)
	if _, err := store.DeleteSource(ctx, event.SourceFile); err != nil {
		t.Fatal(err)
	}
	before, _ := store.VectorSlots(ctx)
	after, err := store.CompactVectors(ctx)
	testcheck.NoError(t, err)
	want := storage.VectorSlots{Slots: blocksFor(3 * vec0BlockSize / 2), Vectors: 3 * vec0BlockSize / 2}
	if before.Slots != 3*vec0BlockSize || after != want {
		t.Fatalf("expected %d positions before and %+v after, got %+v and %+v", 3*vec0BlockSize, want, before, after)
	}
}

func TestCompactVectorsKeepsTheSameNearest(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	saveVectorEvents(t, store, 2*vec0BlockSize)
	if _, err := store.DeleteSource(ctx, event.SourceGit); err != nil {
		t.Fatal(err)
	}
	before := nearestHits(t, store)
	if _, err := store.CompactVectors(ctx); err != nil {
		t.Fatal(err)
	}
	if after := nearestHits(t, store); len(before) == 0 || !reflect.DeepEqual(before, after) {
		t.Fatalf("expected the same top-k after compacting, got %+v then %+v", before, after)
	}
}

func TestCompactVectorsWithoutVectorsDoesNothing(t *testing.T) {
	store := openTestStore(t)
	after, err := store.CompactVectors(context.Background())
	if err != nil || after != (storage.VectorSlots{}) {
		t.Fatalf("expected no blocks and no error, got %+v: %v", after, err)
	}
}

func TestVectorSlotsCountsPositionsAndVectors(t *testing.T) {
	store := openTestStore(t)
	saveVectorEvents(t, store, 3)
	slots, err := store.VectorSlots(context.Background())
	if err != nil || slots != (storage.VectorSlots{Slots: vec0BlockSize, Vectors: 3}) {
		t.Fatalf("expected one block holding 3 vectors, got %+v: %v", slots, err)
	}
}
