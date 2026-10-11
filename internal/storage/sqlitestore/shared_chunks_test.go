package sqlitestore

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// Chunks belong to a text, not to an event (#67).

// visit is a browser event with the page text, days after baseTime.
func visit(uid, text string, days int) event.Event {
	return event.Event{UID: uid, Source: event.SourceBrowser, Timestamp: baseTime.AddDate(0, 0, days), Content: text,
		Metadata: event.Metadata{"url": "https://example.com/" + uid}}
}

func rowCount(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var count int
	testcheck.NoError(t, store.db.QueryRow(query, args...).Scan(&count))
	return count
}

// textRows counts the chunks, vectors and keyword entries.
func textRows(t *testing.T, store *Store) [3]int {
	t.Helper()
	return [3]int{rowCount(t, store, `SELECT COUNT(*) FROM chunks`), rowCount(t, store, `SELECT COUNT(*) FROM chunk_embeddings`),
		rowCount(t, store, `SELECT COUNT(*) FROM chunks_fts`)}
}

func hitUIDs(t *testing.T, store *Store, query storage.SimilarityQuery) []string {
	t.Helper()
	hits, err := store.SearchSimilar(context.Background(), query)
	testcheck.NoError(t, err)
	var uids []string
	for _, hit := range hits {
		uids = append(uids, hit.Event.UID)
	}
	return uids
}

// vectorDates reads the first and last dates of the text's vector.
func vectorDates(t *testing.T, store *Store, text string) (time.Time, time.Time) {
	t.Helper()
	var first, last int64
	testcheck.NoError(t, store.db.QueryRow(`SELECT first_at, last_at FROM chunk_embeddings WHERE chunk_id =
		(SELECT id FROM chunks WHERE content_hash = ?)`, contentHash(text)).Scan(&first, &last))
	return fromUnixMillis(first), fromUnixMillis(last)
}

func TestSameTextIsStoredOnce(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	if got := textRows(t, store); got != [3]int{1, 1, 1} {
		t.Fatalf("expected one chunk, vector and keyword entry, got %v", got)
	}
	first, last := vectorDates(t, store, "pedido 4471")
	if !first.Equal(baseTime) || !last.Equal(baseTime.AddDate(0, 0, 10)) {
		t.Fatalf("expected the vector dated from the first to the last visit, got %s to %s", first, last)
	}
}

// k counts texts: the copies of one page no longer take the slots.
func TestSearchReturnsEveryEventOfANearText(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	mustSave(t, store, visit("c", "frete", 1), []float32{0.8, 0.6})
	got := hitUIDs(t, store, storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 2})
	if !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("expected both visits, newest first, then the next text, got %v", got)
	}
	lexical := lexicalUIDs(t, store, `"pedido"`)
	if len(lexical) != 2 {
		t.Fatalf("expected the keyword hit on both visits, got %v", lexical)
	}
}

// CA9.1: a text visited on days 0 and 10 has no event on day 5; the search
// widens k and finds the text that has.
func TestPeriodFilterChecksTheEventsOfASharedText(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	mustSave(t, store, visit("c", "frete", 5), []float32{0, 1})
	day5 := storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1, From: baseTime.AddDate(0, 0, 5), To: baseTime.AddDate(0, 0, 6)}
	if got := hitUIDs(t, store, day5); !slices.Equal(got, []string{"c"}) {
		t.Fatalf("expected only the day 5 event, got %v", got)
	}
	day10 := storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1, From: baseTime.AddDate(0, 0, 10), To: baseTime.AddDate(0, 0, 11)}
	if got := hitUIDs(t, store, day10); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("expected only the day 10 visit of the shared text, got %v", got)
	}
}

// Privacy: forgetting one of two events keeps the other's chunks and
// drops the forgotten date from the vector; the last one takes it all.
func TestForgetKeepsASharedTextUntilItsLastEvent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	_, err := store.DeleteEvent(ctx, "b")
	testcheck.NoError(t, err)
	chunks, err := store.ChunksFor(ctx, []string{"a"})
	testcheck.NoError(t, err)
	if first, last := vectorDates(t, store, "pedido 4471"); len(chunks["a"]) != 1 || !first.Equal(baseTime) || !last.Equal(baseTime) {
		t.Fatalf("expected a's chunk kept and dated by a alone, got %v, %s to %s", chunks, first, last)
	}
	_, err = store.DeleteEvent(ctx, "a")
	testcheck.NoError(t, err)
	if got := textRows(t, store); got != [3]int{0, 0, 0} {
		t.Fatalf("expected no chunk, vector or keyword entry left, got %v", got)
	}
}

func TestEditMovesAnEventToItsNewText(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	edited := visit("b", "pedido 4471 entregue", 10)
	testcheck.NoError(t, store.UpdateEvent(ctx, edited, whole(edited, []float32{0, 1})))
	vectors := firstVectors(t, store, "a", "b")
	if len(vectors["a"]) != 2 || vectors["a"][0] != 1 || len(vectors["b"]) != 2 || vectors["b"][1] != 1 {
		t.Fatalf("expected a with the old text's vector and b with its new one, got %v", vectors)
	}
	if first, last := vectorDates(t, store, "pedido 4471"); !first.Equal(baseTime) || !last.Equal(baseTime) {
		t.Fatalf("expected the old text dated by a alone, got %s to %s", first, last)
	}
}

func TestForgetSourceDeletesItsTexts(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, sampleEvent("g", event.SourceGit, 0), []float32{0, 1})
	_, err := store.DeleteSource(context.Background(), event.SourceBrowser)
	testcheck.NoError(t, err)
	if got := textRows(t, store); got != [3]int{1, 1, 1} {
		t.Fatalf("expected only the commit's chunk left, got %v", got)
	}
}

// A reindex embeds one event of a text; the others have its chunks.
func TestReindexEmbedsASharedTextOnce(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, visit("a", "pedido 4471", 0), []float32{1, 0})
	mustSave(t, store, visit("b", "pedido 4471", 10), []float32{1, 0})
	testcheck.NoError(t, store.StartReindex(ctx, "outro.gguf"))
	pending, err := store.EventsWithoutEmbedding(ctx, 1)
	testcheck.NoError(t, err)
	testcheck.NoError(t, store.SaveEmbeddings(ctx, []storage.EventEmbedding{{Event: pending[0], Chunks: whole(pending[0], []float32{1, 0})}}))
	left, err := store.CountEventsWithoutEmbedding(ctx)
	testcheck.NoError(t, err)
	if first, last := vectorDates(t, store, "pedido 4471"); left != 0 || !first.Equal(baseTime) || !last.Equal(baseTime.AddDate(0, 0, 10)) {
		t.Fatalf("expected nothing left to embed and both visits dated, got %d left, %s to %s", left, first, last)
	}
}

// Migration 14 keeps the first event's chunks of each text and dates them
// by all its events; it starts from version 4, as a real history would.
func TestShareChunksByTextMigration(t *testing.T) {
	legacy := newLegacyDatabase(t, 4)
	legacy.add(visit("a", "pedido 4471", 0), []float32{1, 0})
	legacy.add(visit("b", "pedido 4471", 10), []float32{1, 0})
	legacy.add(visit("c", "frete", 5), []float32{0, 1})
	legacy.close()
	var backup string
	store, err := OpenWithHooks(context.Background(), legacy.Path, Hooks{BackupCreated: func(path string) { backup = path }})
	testcheck.NoError(t, err)
	defer store.Close()
	slots, err := store.VectorSlots(context.Background())
	testcheck.NoError(t, err)
	if got := textRows(t, store); got != [3]int{2, 2, 2} || slots.Vectors != 2 || backup == "" {
		t.Fatalf("expected 2 texts' chunks, vectors and entries and a backup, got %v, %d vectors, backup %q", got, slots.Vectors, backup)
	}
	if first, last := vectorDates(t, store, "pedido 4471"); !first.Equal(baseTime) || !last.Equal(baseTime.AddDate(0, 0, 10)) {
		t.Fatalf("expected the shared text dated by both visits, got %s to %s", first, last)
	}
	if got := hitUIDs(t, store, storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1}); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("expected both visits found through the one chunk, got %v", got)
	}
}
