// Package rag answers natural-language questions about the user's activity:
// embed the question, retrieve similar events, and have the local model
// answer from those events only (RF4).
package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

// Settings tunes retrieval and generation.
type Settings struct {
	TopK            int
	MaxDistance     float64
	QueryPrefix     string
	MaxAnswerTokens int
}

// Question is a user query with optional filters (CA9.1).
type Question struct {
	Text   string
	Source event.Source
	Days   *timeline.DayRange
	// Criteria (people, direction) cannot be applied inside the vector
	// index; when set, retrieval narrows exactly and then ranks.
	Criteria listing.Criteria
}

// Answer is the generated reply and the events it was grounded on.
type Answer struct {
	Text  string
	Found bool
	// Evidence is every retrieved event, numbered from 1 in the prompt.
	Evidence []storage.ScoredEvent
	// Cited lists the 1-based Evidence numbers the reply references.
	Cited []int
}

// Answerer runs the retrieve-then-generate loop.
type Answerer struct {
	store     storage.EventStore
	embedder  llm.Embedder
	generator llm.Generator
	settings  Settings
	now       func() time.Time
	logger    *slog.Logger
}

// Dependencies groups what an Answerer talks to.
type Dependencies struct {
	Store     storage.EventStore
	Embedder  llm.Embedder
	Generator llm.Generator
	// Now supplies the current time for the prompt so the model can resolve
	// "yesterday"-style references.
	Now    func() time.Time
	Logger *slog.Logger
}

// NewAnswerer wires an Answerer.
//
//	answer, err := rag.NewAnswerer(deps, settings).Answer(ctx, question)
func NewAnswerer(deps Dependencies, settings Settings) *Answerer {
	return &Answerer{
		store: deps.Store, embedder: deps.Embedder, generator: deps.Generator,
		settings: settings, now: deps.Now, logger: deps.Logger,
	}
}

// AnswerStage is a phase of answering, reported so the CLI can show what a
// multi-second answer is waiting on.
type AnswerStage int

const (
	StageSearching AnswerStage = iota
	StageGenerating
)

// AnswerObserver follows an Answer call; nil callbacks are ignored.
type AnswerObserver struct {
	StageStarted func(AnswerStage)
	// PeopleResolved reports which named people matched events and which
	// matched nobody (and were therefore not used as a filter).
	PeopleResolved func(matched, unknown []string)
	Generation     llm.GenerationProgress
}

func (o AnswerObserver) notifyStage(stage AnswerStage) {
	if o.StageStarted != nil {
		o.StageStarted(stage)
	}
}

func (o AnswerObserver) notifyPeople(matched, unknown []string) {
	if o.PeopleResolved != nil && len(matched)+len(unknown) > 0 {
		o.PeopleResolved(matched, unknown)
	}
}

// Answer retrieves evidence for question and generates a grounded reply.
// When nothing relevant is retrieved the model is not called at all.
func (a *Answerer) Answer(ctx context.Context, question Question, observer AnswerObserver) (Answer, error) {
	observer.notifyStage(StageSearching)
	hits, err := a.retrieve(ctx, question, observer)
	if err != nil || len(hits) == 0 {
		return Answer{}, err
	}
	observer.notifyStage(StageGenerating)
	return a.generate(ctx, question, hits, observer.Generation)
}

func (a *Answerer) generate(ctx context.Context, question Question, hits []storage.ScoredEvent, progress llm.GenerationProgress) (Answer, error) {
	started := time.Now()
	reply, err := a.generator.Generate(ctx, buildPrompt(question.Text, hits, a.now()), a.settings.MaxAnswerTokens, progress)
	if err != nil {
		return Answer{}, fmt.Errorf("generate answer from %d events: %w", len(hits), err)
	}
	a.logger.Debug("model reply", "evidence_count", len(hits), "duration_ms", time.Since(started).Milliseconds(), "reply", reply)
	if isNotFoundReply(reply) {
		return Answer{}, nil
	}
	return Answer{Text: strings.TrimSpace(reply), Found: true, Evidence: hits, Cited: citedIndexes(reply, len(hits))}, nil
}

func (a *Answerer) retrieve(ctx context.Context, question Question, observer AnswerObserver) ([]storage.ScoredEvent, error) {
	embedding, err := a.embedQuery(question.Text)
	if err != nil {
		return nil, err
	}
	if !question.Criteria.IsEmpty() {
		return a.retrieveAmong(ctx, question, embedding, observer)
	}
	hits, err := a.store.SearchSimilar(ctx, similarityQuery(embedding, question, a.settings.TopK))
	if err != nil {
		return nil, err
	}
	a.logHits(hits)
	if question.IsScoped() {
		return hits, nil
	}
	return withinDistance(hits, a.settings.MaxDistance), nil
}

// IsScoped reports whether the user narrowed the search by source or date.
// Scoped questions skip the distance cutoff: generic questions such as
// "what did I do?" are far from every event in embedding space, and the
// explicit scope is already the relevance signal. The model's
// SEM_INFORMACAO reply still guards against unrelated evidence.
func (q Question) IsScoped() bool {
	return q.Source != "" || q.Days != nil || !q.Criteria.IsEmpty()
}

func (a *Answerer) embedQuery(text string) ([]float32, error) {
	embedding, err := a.embedder.Embed(a.settings.QueryPrefix + text)
	if err != nil {
		return nil, fmt.Errorf("embed question %q: %w", text, err)
	}
	return embedding, nil
}

// logHits records every candidate with its distance, which is what one
// needs to tune retrieval.max_distance.
func (a *Answerer) logHits(hits []storage.ScoredEvent) {
	for _, hit := range hits {
		a.logger.Debug("retrieval candidate", "uid", hit.Event.UID, "source", hit.Event.Source,
			"distance", hit.Distance, "kept", hit.Distance <= a.settings.MaxDistance, "headline", hit.Event.Headline())
	}
}

func similarityQuery(embedding []float32, question Question, limit int) storage.SimilarityQuery {
	query := storage.SimilarityQuery{Embedding: embedding, Limit: limit, Source: question.Source}
	if question.Days != nil {
		query.From, query.To = question.Days.Start(), question.Days.End()
	}
	return query
}

// withinDistance drops weak matches; hits arrive sorted by distance.
func withinDistance(hits []storage.ScoredEvent, maxDistance float64) []storage.ScoredEvent {
	for i, hit := range hits {
		if hit.Distance > maxDistance {
			return hits[:i]
		}
	}
	return hits
}
