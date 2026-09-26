package rag

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/timeline"
)

var fixedNow = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func scored(uid string, source event.Source, distance float64) storage.ScoredEvent {
	return storage.ScoredEvent{
		Event:    event.Event{UID: uid, Source: source, Timestamp: fixedNow.Add(-time.Hour), Content: "content of " + uid},
		Distance: distance,
	}
}

func newTestAnswerer(store *testfakes.FakeEventStore, generator *testfakes.FakeGenerator) (*Answerer, *testfakes.FakeEmbedder) {
	embedder := &testfakes.FakeEmbedder{}
	settings := Settings{TopK: 5, MaxDistance: 0.5, QueryPrefix: "q: ", MaxAnswerTokens: 100}
	deps := Dependencies{
		Store: store, Embedder: embedder, Generator: generator,
		Now: func() time.Time { return fixedNow }, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	return NewAnswerer(deps, settings), embedder
}

func storeWithHits(hits ...storage.ScoredEvent) *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	store.SearchResults = hits
	return store
}

func TestAnswerReturnsGroundedReplyWithCitations(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: "Você corrigiu o login [1] e leu docs [2]."}
	answerer, _ := newTestAnswerer(storeWithHits(scored("a", event.SourceGit, 0.1), scored("b", event.SourceBrowser, 0.2)), generator)
	answer, err := answerer.Answer(context.Background(), Question{Text: "o que fiz?"}, AnswerObserver{})
	if err != nil || !answer.Found || len(answer.Evidence) != 2 || len(answer.Cited) != 2 {
		t.Fatalf("expected found answer citing both events, got %+v (err %v)", answer, err)
	}
}

func TestAnswerWithoutRelevantEventsSkipsGeneration(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: "invented"}
	answerer, _ := newTestAnswerer(storeWithHits(scored("far", event.SourceGit, 0.9)), generator)
	answer, err := answerer.Answer(context.Background(), Question{Text: "receita de bolo?"}, AnswerObserver{})
	if err != nil || answer.Found || generator.Calls != 0 {
		t.Fatalf("expected not found without calling the model, got %+v, %d calls (err %v)", answer, generator.Calls, err)
	}
}

func TestScopedQuestionKeepsDistantHits(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: "Você commitou [1]."}
	answerer, _ := newTestAnswerer(storeWithHits(scored("far", event.SourceGit, 0.9)), generator)
	answer, _ := answerer.Answer(context.Background(), Question{Text: "o que fiz?", Source: event.SourceGit}, AnswerObserver{})
	if !answer.Found || generator.Calls != 1 {
		t.Fatalf("expected scoped question to reach the model, got %+v with %d calls", answer, generator.Calls)
	}
}

func TestIsScoped(t *testing.T) {
	days, _ := timeline.ParseDayRange("hoje", "", fixedNow)
	if (Question{}).IsScoped() || !(Question{Days: &days}).IsScoped() || !(Question{Source: event.SourceGit}).IsScoped() {
		t.Fatal("expected only questions with a source or days to be scoped")
	}
}

func TestAnswerHonoursModelNotFoundMarker(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: " " + NotFoundMarker + "\n"}
	answerer, _ := newTestAnswerer(storeWithHits(scored("a", event.SourceGit, 0.1)), generator)
	answer, _ := answerer.Answer(context.Background(), Question{Text: "x"}, AnswerObserver{})
	if answer.Found {
		t.Fatal("expected the not-found marker to produce a not-found answer")
	}
}

func TestAnswerPassesFiltersToSearch(t *testing.T) {
	store := storeWithHits()
	answerer, embedder := newTestAnswerer(store, &testfakes.FakeGenerator{})
	days, _ := timeline.ParseDayRange("2026-09-20", "2026-09-26", fixedNow)
	answerer.Answer(context.Background(), Question{Text: "sqlite", Source: event.SourceBrowser, Days: &days}, AnswerObserver{})
	query := store.LastQuery
	if query.Source != event.SourceBrowser || !query.From.Equal(days.Start()) || !query.To.Equal(days.End()) || query.Limit != 5 {
		t.Fatalf("expected filters in query, got %+v", query)
	}
	if embedder.Inputs[0] != "q: sqlite" {
		t.Fatalf("expected query prefix, got %q", embedder.Inputs[0])
	}
}

func TestAnswerWrapsGeneratorError(t *testing.T) {
	generator := &testfakes.FakeGenerator{FailWith: errors.New("out of memory")}
	answerer, _ := newTestAnswerer(storeWithHits(scored("a", event.SourceGit, 0.1)), generator)
	if _, err := answerer.Answer(context.Background(), Question{Text: "x"}, AnswerObserver{}); err == nil || !strings.Contains(err.Error(), "out of memory") {
		t.Fatalf("expected wrapped generator error, got %v", err)
	}
}

func TestAnswerReportsStagesAndStreamsReply(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: "Você fez [1]."}
	answerer, _ := newTestAnswerer(storeWithHits(scored("a", event.SourceGit, 0.1)), generator)
	var stages []AnswerStage
	var streamed string
	observer := AnswerObserver{
		StageStarted: func(stage AnswerStage) { stages = append(stages, stage) },
		Generation:   llm.GenerationProgress{TokenGenerated: func(piece string) { streamed += piece }},
	}
	answerer.Answer(context.Background(), Question{Text: "o que fiz?"}, observer)
	if len(stages) != 2 || stages[0] != StageSearching || stages[1] != StageGenerating || streamed != "Você fez [1]." {
		t.Fatalf("unexpected stages %v / stream %q", stages, streamed)
	}
}

func TestAnswerSkipsGeneratingStageWithoutEvidence(t *testing.T) {
	answerer, _ := newTestAnswerer(storeWithHits(), &testfakes.FakeGenerator{})
	var stages []AnswerStage
	answerer.Answer(context.Background(), Question{Text: "x"}, AnswerObserver{StageStarted: func(stage AnswerStage) { stages = append(stages, stage) }})
	if len(stages) != 1 || stages[0] != StageSearching {
		t.Fatalf("expected only the search stage, got %v", stages)
	}
}

func TestAnswerPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	answerer, _ := newTestAnswerer(storeWithHits(scored("a", event.SourceGit, 0.1)), &testfakes.FakeGenerator{Reply: "x"})
	if _, err := answerer.Answer(ctx, Question{Text: "x"}, AnswerObserver{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSimilarityQueryWithoutDaysIsUnbounded(t *testing.T) {
	query := similarityQuery([]float32{1}, Question{Text: "x"}, 3)
	if !query.From.IsZero() || !query.To.IsZero() || query.Limit != 3 {
		t.Fatalf("expected unbounded query with limit 3, got %+v", query)
	}
}

func TestWithinDistanceCutsAtThreshold(t *testing.T) {
	hits := withinDistance([]storage.ScoredEvent{scored("a", "", 0.1), scored("b", "", 0.5), scored("c", "", 0.51)}, 0.5)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits within 0.5, got %d", len(hits))
	}
}
