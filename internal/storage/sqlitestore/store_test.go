package sqlitestore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

var baseTime = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func sampleEvent(uid string, source event.Source, offset time.Duration) event.Event {
	return event.Event{
		UID: uid, Source: source, Timestamp: baseTime.Add(offset),
		Content: "content " + uid, Metadata: event.Metadata{"key": uid},
	}
}

func mustSave(t *testing.T, store *Store, ev event.Event, embedding []float32) {
	t.Helper()
	if _, err := store.SaveEvent(context.Background(), ev, whole(ev, embedding)); err != nil {
		t.Fatalf("save %q: %v", ev.UID, err)
	}
}

func TestSaveEventDeduplicatesByUID(t *testing.T) {
	store := openTestStore(t)
	ev := sampleEvent("a", event.SourceGit, 0)
	first, _ := store.SaveEvent(context.Background(), ev, whole(ev, []float32{1, 0}))
	second, err := store.SaveEvent(context.Background(), ev, whole(ev, []float32{1, 0}))
	if err != nil || !first || second {
		t.Fatalf("expected first insert true and second false, got %v, %v (err %v)", first, second, err)
	}
}

func TestStoredEvent(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), nil)
	stored, found, err := store.StoredEvent(context.Background(), "a")
	_, missing, _ := store.StoredEvent(context.Background(), "b")
	if err != nil || !found || missing || stored.UID != "a" || stored.Content != sampleEvent("a", event.SourceGit, 0).Content {
		t.Fatalf("expected a found with its content and b missing, got %+v %v %v (err %v)", stored, found, missing, err)
	}
}

func TestDeleteEventRemovesPrivateRowsAndRemembersUID(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	ev := event.Event{UID: "private-uid", Source: event.SourceTeams, Timestamp: baseTime, Content: "unique-private-message", Metadata: event.Message{Sender: "Private Person", Kind: event.KindChat}.Metadata()}
	mustSave(t, store, ev, []float32{1, 0})
	deleted, err := store.DeleteEvent(ctx, ev.UID)
	if err != nil || !deleted {
		t.Fatalf("delete event: %v, %v", deleted, err)
	}
	for _, table := range []string{"events", "chunks", "chunk_embeddings", "chunks_fts", "event_people", "file_modifications"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Errorf("%s retained %d rows (err %v)", table, count, err)
		}
	}
	forgotten, err := store.IsForgotten(ctx, ev.UID)
	if err != nil || !forgotten {
		t.Fatalf("UID not retained for suppression: %v, %v", forgotten, err)
	}
}

func TestUpdateEventReplacesContentAndEmbedding(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	edited := sampleEvent("a", event.SourceGit, 0)
	edited.Content = "texto editado"
	if err := store.UpdateEvent(context.Background(), edited, whole(edited, []float32{0, 1})); err != nil {
		t.Fatal(err)
	}
	stored, _, _ := store.StoredEvent(context.Background(), "a")
	vectors := firstVectors(t, store, "a")
	if stored.Content != "texto editado" || len(vectors["a"]) != 2 || vectors["a"][1] != 1 {
		t.Fatalf("expected the new content and vector, got %q %v", stored.Content, vectors["a"])
	}
}

func TestUpdateEventWithoutEmbeddingRemovesVector(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	emptied := sampleEvent("a", event.SourceGit, 0)
	emptied.Content = ""
	testcheck.NoError(t, store.UpdateEvent(context.Background(), emptied, nil))
	if vectors := firstVectors(t, store, "a"); len(vectors) != 0 {
		t.Fatalf("expected no vector left, got %v", vectors)
	}
}

func TestUpdateEventRejectsUnknownUID(t *testing.T) {
	store := openTestStore(t)
	if err := store.UpdateEvent(context.Background(), sampleEvent("zz", event.SourceGit, 0), nil); err == nil || !strings.Contains(err.Error(), `"zz"`) {
		t.Fatalf("expected an error naming the uid, got %v", err)
	}
}

func TestEventsBetweenIsChronologicalAndHalfOpen(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("late", event.SourceFile, 2*time.Hour), nil)
	mustSave(t, store, sampleEvent("early", event.SourceGit, time.Hour), nil)
	mustSave(t, store, sampleEvent("excluded", event.SourceGit, 3*time.Hour), nil)
	events, err := store.EventsBetween(context.Background(), baseTime, baseTime.Add(3*time.Hour))
	if err != nil || len(events) != 2 || events[0].UID != "early" || events[1].UID != "late" {
		t.Fatalf("expected [early late], got %+v (err %v)", events, err)
	}
}

func TestEventsBetweenRoundTripsFields(t *testing.T) {
	store := openTestStore(t)
	ev := sampleEvent("a", event.SourceBrowser, 0)
	mustSave(t, store, ev, nil)
	events, _ := store.EventsBetween(context.Background(), baseTime, baseTime.Add(time.Minute))
	got := events[0]
	if !got.Timestamp.Equal(ev.Timestamp) || got.Source != ev.Source || got.Metadata["key"] != "a" || got.Content != ev.Content {
		t.Fatalf("round trip mismatch: saved %+v, loaded %+v", ev, got)
	}
}

func TestSaveEventRejectsDimensionChange(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	_, err := store.SaveEvent(context.Background(), sampleEvent("b", event.SourceGit, 0), whole(sampleEvent("b", event.SourceGit, 0), []float32{1, 0, 0}))
	if err == nil {
		t.Fatal("expected an error when embedding dimensions change")
	}
}

func TestSaveEventRollsBackEventWhenEmbeddingFails(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	if _, err := store.SaveEvent(context.Background(), sampleEvent("b", event.SourceGit, 0), whole(sampleEvent("b", event.SourceGit, 0), []float32{1, 0, 0})); err == nil {
		t.Fatal("expected the wrong-sized embedding to be refused")
	}
	if _, found, _ := store.StoredEvent(context.Background(), "b"); found {
		t.Fatal("event must not persist without its embedding, or re-ingestion would skip it forever")
	}
}

func TestSearchSimilarWithoutVectorsReturnsNothing(t *testing.T) {
	store := openTestStore(t)
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 3})
	if err != nil || len(hits) != 0 {
		t.Fatalf("expected no hits and no error, got %v (err %v)", hits, err)
	}
}

func TestSearchSimilarOrdersByDistance(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("far", event.SourceGit, 0), []float32{0, 1})
	mustSave(t, store, sampleEvent("near", event.SourceGit, 0), []float32{1, 0.1})
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 2})
	if err != nil || len(hits) != 2 || hits[0].Event.UID != "near" || hits[0].Distance >= hits[1].Distance {
		t.Fatalf("expected near before far, got %+v (err %v)", hits, err)
	}
}

// CA9.1: the filter must be applied inside the KNN scan. The matching event
// is the *least* similar one, so post-filtering the top-1 would lose it.
func TestSearchSimilarAppliesSourceFilterBeforeLimit(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("git-near", event.SourceGit, 0), []float32{1, 0})
	mustSave(t, store, sampleEvent("browser-far", event.SourceBrowser, 0), []float32{0, 1})
	query := storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1, Source: event.SourceBrowser}
	hits, err := store.SearchSimilar(context.Background(), query)
	if err != nil || len(hits) != 1 || hits[0].Event.UID != "browser-far" {
		t.Fatalf("expected only browser-far, got %+v (err %v)", hits, err)
	}
}

func TestSearchSimilarAppliesTimeFilterBeforeLimit(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("old-near", event.SourceGit, -48*time.Hour), []float32{1, 0})
	mustSave(t, store, sampleEvent("recent-far", event.SourceGit, 0), []float32{0, 1})
	query := storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1, From: baseTime.Add(-time.Hour), To: baseTime.Add(time.Hour)}
	hits, err := store.SearchSimilar(context.Background(), query)
	if err != nil || len(hits) != 1 || hits[0].Event.UID != "recent-far" {
		t.Fatalf("expected only recent-far, got %+v (err %v)", hits, err)
	}
}

func TestSearchSimilarPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	first, _ := Open(context.Background(), path)
	mustSave(t, first, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	first.Close()
	second, _ := Open(context.Background(), path)
	defer second.Close()
	hits, err := second.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 1})
	if err != nil || len(hits) != 1 {
		t.Fatalf("expected 1 hit after reopen, got %d (err %v)", len(hits), err)
	}
}

func TestDeleteSourceRemovesEventsAndEmbeddings(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("teams-1", event.SourceTeams, 0), []float32{1, 0})
	mustSave(t, store, sampleEvent("git-1", event.SourceGit, 0), []float32{0, 1})
	removed, err := store.DeleteSource(context.Background(), event.SourceTeams)
	if err != nil || removed != 1 {
		t.Fatalf("expected 1 removed, got %d (err %v)", removed, err)
	}
	hits, _ := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 5})
	if len(hits) != 1 || hits[0].Event.UID != "git-1" {
		t.Fatalf("expected only the git embedding to remain, got %+v", hits)
	}
}

func TestDeleteSourceAllowsReingest(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("teams-1", event.SourceTeams, 0), []float32{1, 0})
	if _, err := store.DeleteSource(context.Background(), event.SourceTeams); err != nil {
		t.Fatal(err)
	}
	inserted, err := store.SaveEvent(context.Background(), sampleEvent("teams-1", event.SourceTeams, 0), whole(sampleEvent("teams-1", event.SourceTeams, 0), []float32{1, 0}))
	if err != nil || !inserted {
		t.Fatalf("expected the event to be insertable again, got %v (err %v)", inserted, err)
	}
}

func TestDeleteSourceWithoutVectorTable(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("f", event.SourceFile, 0), nil)
	if removed, err := store.DeleteSource(context.Background(), event.SourceFile); err != nil || removed != 1 {
		t.Fatalf("expected 1 removed, got %d (err %v)", removed, err)
	}
}

func TestEmbeddingsForReturnsStoredVectors(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceTeams, 0), []float32{0.6, -0.8})
	mustSave(t, store, sampleEvent("no-vector", event.SourceFile, 0), nil)
	embeddings := firstVectors(t, store, "a", "no-vector", "missing")
	var err error
	if err != nil || len(embeddings) != 1 || !sameDirection(embeddings["a"], []float32{0.6, -0.8}) {
		t.Fatalf("expected only a's vector, got %v (err %v)", embeddings, err)
	}
}

func TestEmbeddingsForWithoutVectorTable(t *testing.T) {
	store := openTestStore(t)
	embeddings := firstVectors(t, store, "a")
	var err error
	if err != nil || len(embeddings) != 0 {
		t.Fatalf("expected an empty map, got %v (err %v)", embeddings, err)
	}
}

func TestDecodeFloat32sRejectsTruncatedBlob(t *testing.T) {
	if _, err := decodeFloat32s([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected an error for a 3-byte blob")
	}
}

func TestOpenCreatesOwnerOnlyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), nil)
	for _, suffix := range []string{"", "-wal"} {
		info, err := os.Stat(path + suffix)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("expected %s to be 0600, got %v (err %v)", path+suffix, info.Mode().Perm(), err)
		}
	}
}

// Regression: databases created by earlier versions were world-readable.
func TestOpenRestrictsExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	testcheck.NoError(t, os.WriteFile(path, nil, 0o644))
	testcheck.NoError(t, os.WriteFile(path+"-shm", nil, 0o644))
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, suffix := range []string{"", "-shm"} {
		if info, _ := os.Stat(path + suffix); info != nil && info.Mode().Perm() != 0o600 {
			t.Fatalf("expected %s tightened to 0600, got %v", path+suffix, info.Mode().Perm())
		}
	}
}

func TestSecureDeleteIsOn(t *testing.T) {
	store := openTestStore(t)
	var mode int
	if err := store.db.QueryRow(`PRAGMA secure_delete`).Scan(&mode); err != nil || mode != 1 {
		t.Fatalf("expected secure_delete on, got %d (err %v)", mode, err)
	}
}

// forget must remove the text from the file, not only unlink it.
func TestDeleteSourceLeavesNoTextOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, _ := Open(context.Background(), path)
	secret := sampleEvent("a", event.SourceTeams, 0)
	secret.Content = "senha-do-cofre-8472"
	mustSave(t, store, secret, []float32{1, 0})
	if _, err := store.DeleteSource(context.Background(), event.SourceTeams); err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, suffix := range []string{"", "-wal"} {
		raw, _ := os.ReadFile(path + suffix)
		if bytes.Contains(raw, []byte("senha-do-cofre-8472")) {
			t.Fatalf("deleted text still in %s", path+suffix)
		}
	}
}

func TestStoredChunksForContentFindsIdenticalText(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first, second := sampleEvent("a", event.SourceBrowser, 0), sampleEvent("b", event.SourceBrowser, 1)
	mustSave(t, store, first, []float32{0, 1})
	mustSave(t, store, second, nil)
	chunks, found, err := store.StoredChunksForContent(ctx, first.Content)
	if err != nil || !found || len(chunks) != 1 || chunks[0].Vector[1] != 1 || chunks[0].End != len(first.Content) {
		t.Fatalf("expected the stored chunk for identical text, got %+v %v (err %v)", chunks, found, err)
	}
	if _, found, _ := store.StoredChunksForContent(ctx, "texto novo"); found {
		t.Fatal("expected no chunks for unseen text")
	}
}

// Migration 3 hashes events stored before it existed.
func TestContentHashMigrationFillsOldEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	legacy := openRaw(t, path)
	if _, err := legacy.Exec(createEventsTable); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO events (uid, occurred_at, source, content, metadata) VALUES ('a', 0, 'git', 'antigo', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var hash string
	testcheck.NoError(t, store.db.QueryRow(`SELECT content_hash FROM events WHERE uid = 'a'`).Scan(&hash))
	if hash != contentHash("antigo") {
		t.Fatalf("expected the old event hashed, got %q", hash)
	}
}

// whole is a single chunk covering all of ev's text, as a short event is
// stored; nil vector means no chunk.
func whole(ev event.Event, vector []float32) []storage.Chunk {
	if vector == nil {
		return nil
	}
	return []storage.Chunk{{End: len(ev.Content), Vector: vector}}
}

// firstVectors reads each event's first chunk vector.
func firstVectors(t testing.TB, store *Store, uids ...string) map[string][]float32 {
	t.Helper()
	chunks, err := store.ChunksFor(context.Background(), uids)
	if err != nil {
		t.Fatal(err)
	}
	vectors := map[string][]float32{}
	for uid, eventChunks := range chunks {
		vectors[uid] = eventChunks[0].Vector
	}
	return vectors
}
