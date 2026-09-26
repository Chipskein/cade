package rag

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
)

// When a question names people or a direction, the vector index cannot
// enforce them: the events are first narrowed exactly, then ranked by
// similarity among themselves. Stored and query vectors are unit length,
// so cosine distance is 1 - dot product, as in sqlite-vec.

// retrieveAmong returns the top-k events that satisfy all filters.
func (a *Answerer) retrieveAmong(ctx context.Context, query queryplan.Query, embedding []float32, observer AnswerObserver) ([]storage.ScoredEvent, error) {
	candidates, err := a.candidates(ctx, query, observer)
	if err != nil {
		return nil, err
	}
	ranked, err := a.rank(ctx, embedding, candidates)
	if err != nil {
		return nil, err
	}
	ranked = collapseRepeats(withoutRemoved(withoutChatter(ranked)))
	return ranked[:min(len(ranked), a.settings.TopK)], nil
}

// candidates loads the question's period (or all time) and applies the
// source and criteria in memory; a person's events over a period are few.
func (a *Answerer) candidates(ctx context.Context, query queryplan.Query, observer AnswerObserver) ([]event.Event, error) {
	from, to := time.Unix(0, 0), a.now().AddDate(1, 0, 0)
	if query.Days != nil {
		from, to = query.Days.Start(), query.Days.End()
	}
	events, err := a.store.EventsBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	events = keepSource(events, query.Source)
	kept, matched, unknown := query.Criteria.Apply(events)
	observer.notifyPeople(matched, unknown)
	return kept, nil
}

func keepSource(events []event.Event, source event.Source) []event.Event {
	if source == "" {
		return events
	}
	var kept []event.Event
	for _, ev := range events {
		if ev.Source == source {
			kept = append(kept, ev)
		}
	}
	return kept
}

// rank orders events by distance to embedding; events without a stored
// embedding (files without text) cannot be ranked and are left out.
func (a *Answerer) rank(ctx context.Context, embedding []float32, events []event.Event) ([]storage.ScoredEvent, error) {
	uids := make([]string, len(events))
	for i, ev := range events {
		uids[i] = ev.UID
	}
	chunks, err := a.store.ChunksFor(ctx, uids)
	if err != nil {
		return nil, err
	}
	var ranked []storage.ScoredEvent
	for _, ev := range events {
		if eventChunks, found := chunks[ev.UID]; found {
			ranked = append(ranked, closestChunk(embedding, ev, eventChunks))
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Distance < ranked[j].Distance })
	return ranked, nil
}

// closestChunk scores an event by its best chunk: a long note matches when
// any part of it does.
func closestChunk(embedding []float32, ev event.Event, chunks []storage.Chunk) storage.ScoredEvent {
	best := storage.ScoredEvent{Event: ev, Distance: math.Inf(1), ChunkCount: len(chunks)}
	for _, chunk := range chunks {
		if distance := cosineDistance(embedding, chunk.Vector); distance < best.Distance {
			best.Distance, best.Chunk = distance, storage.Chunk{Ordinal: chunk.Ordinal, Start: chunk.Start, End: chunk.End}
		}
	}
	return best
}

func cosineDistance(a, b []float32) float64 {
	var dot float64
	for i := range min(len(a), len(b)) {
		dot += float64(a[i]) * float64(b[i])
	}
	return 1 - dot
}

// FilterByTopic keeps the events close enough to topic (within
// max_distance), in chronological order. It narrows a listing such as
// "páginas que visitei hoje sobre redis" to the pages about redis.
//
//	pages, err := answerer.FilterByTopic(ctx, "redis", todaysPages)
func (a *Answerer) FilterByTopic(ctx context.Context, topic string, events []event.Event) ([]event.Event, error) {
	embedding, err := a.embedQuery(topic)
	if err != nil {
		return nil, err
	}
	ranked, err := a.rank(ctx, embedding, events)
	if err != nil {
		return nil, err
	}
	var kept []event.Event
	for _, hit := range withinDistance(ranked, a.settings.MaxDistance) {
		kept = append(kept, hit.Event)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Timestamp.Before(kept[j].Timestamp) })
	return kept, nil
}
