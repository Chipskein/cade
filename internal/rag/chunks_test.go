package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

func chunkHit(uid string, ordinal int, distance float64) storage.ScoredEvent {
	return storage.ScoredEvent{Event: event.Event{UID: uid, Source: event.SourceFile, Content: "x"}, Distance: distance,
		Chunk: storage.Chunk{Ordinal: ordinal}, ChunkCount: 3}
}

func TestBestChunkPerEventKeepsClosest(t *testing.T) {
	hits := bestChunkPerEvent([]storage.ScoredEvent{chunkHit("a", 2, 0.1), chunkHit("b", 0, 0.2), chunkHit("a", 0, 0.3)})
	if len(hits) != 2 || hits[0].Chunk.Ordinal != 2 || hits[0].Repeats != 0 {
		t.Fatalf("expected a's closest chunk once, not counted as a repeat, got %+v", hits)
	}
}

// A person question ranks each event by its best chunk.
func TestRankUsesClosestChunk(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	note := teamsMessage("nota", "Marcos Lima", 9)
	store.Events = []event.Event{note}
	store.Chunks = map[string][]storage.Chunk{"nota": {{Ordinal: 0, Start: 0, End: 5, Vector: []float32{0, 1}}, {Ordinal: 1, Start: 5, End: 9, Vector: []float32{1, 0}}}}
	answerer, _ := newTestAnswerer(store, &testfakes.FakeGenerator{})
	answerer.embedder = &testfakes.FakeEmbedder{Vector: []float32{1, 0}}
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "q", Criteria: listing.Criteria{People: []string{"Marcos"}}}, AnswerObserver{})
	if err != nil || len(hits) != 1 || hits[0].Chunk.Ordinal != 1 || hits[0].Distance > 1e-6 || hits[0].ChunkCount != 2 {
		t.Fatalf("expected the second chunk at distance 0, got %+v (err %v)", hits, err)
	}
}

// The prompt shows the chunk that matched, with its position.
func TestPromptShowsMatchedChunk(t *testing.T) {
	content := "arquitetura.md\n" + strings.Repeat("filler ", 300) + "O timeout foi aumentado para 45 segundos."
	start := strings.Index(content, "O timeout")
	hit := storage.ScoredEvent{Event: event.Event{Source: event.SourceFile, Timestamp: fixedNow, Content: content,
		Metadata: event.File{Path: "/notas/arquitetura.md"}.Metadata()}, Chunk: storage.Chunk{Ordinal: 2, Start: start, End: len(content)}, ChunkCount: 3}
	prompt := formatEvidence([]storage.ScoredEvent{hit}, time.UTC)
	if !strings.Contains(prompt, "(arquitetura.md, trecho 3 de 3)") || !strings.Contains(prompt, "45 segundos") || strings.Contains(prompt, "filler filler") {
		t.Fatalf("expected the matched chunk and its position, got %q", prompt)
	}
}
