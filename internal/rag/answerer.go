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

	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
)

// Settings tunes retrieval and generation.
type Settings struct {
	TopK            int
	MaxDistance     float64
	MaxBestDistance float64
	QueryPrefix     string
	MaxAnswerTokens int
}

// Answer is the generated reply and the events it was grounded on.
type Answer struct {
	Text  string
	Found bool
	// Evidence is every retrieved event, numbered from 1 in the prompt.
	Evidence []storage.ScoredEvent
	// Cited lists the 1-based Evidence numbers the reply references.
	Cited []int
	// UnknownCitations are numbers the reply cites that match no evidence:
	// the claim next to them has no source.
	UnknownCitations []int
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
func (a *Answerer) Answer(ctx context.Context, query queryplan.Query, observer AnswerObserver) (Answer, error) {
	observer.notifyStage(StageSearching)
	hits, err := a.Retrieve(ctx, query, observer)
	if err != nil || len(hits) == 0 {
		return Answer{}, err
	}
	observer.notifyStage(StageGenerating)
	return a.generate(ctx, query, hits, observer.Generation)
}

func (a *Answerer) generate(ctx context.Context, query queryplan.Query, hits []storage.ScoredEvent, progress llm.GenerationProgress) (Answer, error) {
	started := time.Now()
	reply, err := a.generator.Generate(ctx, buildPrompt(query.Question, hits, a.now()), a.settings.MaxAnswerTokens, progress)
	if err != nil {
		return Answer{}, fmt.Errorf("generate answer from %d events: %w", len(hits), err)
	}
	a.logger.Debug("model reply", "evidence_count", len(hits), "duration_ms", time.Since(started).Milliseconds(), "reply", reply)
	if isNotFoundReply(reply) {
		return Answer{}, nil
	}
	cited, unknown := citedIndexes(reply, len(hits))
	return Answer{Text: strings.TrimSpace(reply), Found: true, Evidence: hits, Cited: cited, UnknownCitations: unknown}, nil
}

// Retrieve returns the evidence Answer would give the model, without
// generating; the retrieval suite measures it directly. It embeds the
// query's semantic text. Criteria (people, direction)
// cannot be applied inside the vector index; when set, retrieval narrows
// exactly and then ranks. Scoped queries skip the distance cutoff: generic
// questions such as "what did I do?" are far from every event in embedding
// space, and the explicit scope is already the relevance signal. The
// model's SEM_INFORMACAO reply still guards against unrelated evidence.
func (a *Answerer) Retrieve(ctx context.Context, query queryplan.Query, observer AnswerObserver) ([]storage.ScoredEvent, error) {
	embedding, err := a.embedQuery(searchText(query))
	if err != nil {
		return nil, err
	}
	if !query.Criteria.IsEmpty() {
		return a.retrieveAmong(ctx, query, embedding, observer)
	}
	hits, err := a.store.SearchSimilar(ctx, similarityQuery(embedding, query, a.settings.TopK*chatterHeadroom))
	if err != nil {
		return nil, err
	}
	hits = withoutChatter(hits)
	hits = hits[:min(len(hits), a.settings.TopK)]
	a.logHits(hits)
	if query.IsScoped() {
		return hits, nil
	}
	return a.relevantHits(hits), nil
}

// relevantHits answers an unfiltered question only if its closest event is
// within max_best_distance, then keeps the hits within max_distance. On the
// retrieval suite, the closest event of every answerable question was at
// most 0.60 and of every unanswerable one at least 0.64, while single hits
// of both overlapped around 0.64-0.66: one cutoff per hit cannot separate
// them. Hits arrive sorted by distance.
func (a *Answerer) relevantHits(hits []storage.ScoredEvent) []storage.ScoredEvent {
	if len(hits) == 0 {
		return nil
	}
	if a.settings.MaxBestDistance > 0 && hits[0].Distance > a.settings.MaxBestDistance {
		a.logger.Debug("question rejected: closest event too far", "closest_distance", hits[0].Distance, "max_best_distance", a.settings.MaxBestDistance)
		return nil
	}
	return withinDistance(hits, a.settings.MaxDistance)
}

// searchText falls back to the question for queries built without Resolve.
func searchText(query queryplan.Query) string {
	if query.SemanticText != "" {
		return query.SemanticText
	}
	return query.Question
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

func similarityQuery(embedding []float32, query queryplan.Query, limit int) storage.SimilarityQuery {
	similarity := storage.SimilarityQuery{Embedding: embedding, Limit: limit, Source: query.Source}
	if query.Days != nil {
		similarity.From, similarity.To = query.Days.Start(), query.Days.End()
	}
	return similarity
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
