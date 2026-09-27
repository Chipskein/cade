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
// enforce them: the store selects the matching events exactly (in SQL, no
// content read), and they are ranked by similarity among themselves.
// Stored and query vectors are unit length, so cosine distance is 1 - dot
// product, as in sqlite-vec.

// retrieveAmong returns the top-k events that satisfy all filters.
func (a *Answerer) retrieveAmong(ctx context.Context, query queryplan.Query, embedding []float32, observer AnswerObserver) ([]storage.ScoredEvent, error) {
	filter, err := a.resolveFilter(ctx, query, observer)
	if err != nil {
		return nil, err
	}
	tooMany, err := a.tooManyToRank(ctx, filter)
	if err != nil {
		return nil, err
	}
	if tooMany {
		return a.searchAmong(ctx, embedding, query, filter)
	}
	return a.rankAmong(ctx, embedding, query, filter)
}

// rankAmong loads the matching events and ranks every one of them.
func (a *Answerer) rankAmong(ctx context.Context, embedding []float32, query queryplan.Query, filter storage.EventFilter) ([]storage.ScoredEvent, error) {
	candidates, err := a.store.EventsMatching(ctx, filter)
	if err != nil {
		return nil, err
	}
	ranked, err := a.rank(ctx, embedding, candidates)
	if err != nil {
		return nil, err
	}
	ranked = usableEvidence(ranked, query)
	return ranked[:min(len(ranked), a.settings.TopK)], nil
}

// resolveFilter is the question's period (or all time), source and
// criteria, with each name resolved against the stored events.
func (a *Answerer) resolveFilter(ctx context.Context, query queryplan.Query, observer AnswerObserver) (storage.EventFilter, error) {
	scope := storage.EventFilter{From: time.Unix(0, 0), To: a.now().AddDate(1, 0, 0), Source: query.Source}
	if query.Days != nil {
		scope.From, scope.To = query.Days.Start(), query.Days.End()
	}
	filter, resolution, err := storage.ResolveCriteria(ctx, a.store, scope, query.Criteria)
	if err != nil {
		return storage.EventFilter{}, err
	}
	observer.notifyPeople(resolution.Matched, resolution.Unknown)
	return filter, nil
}

// tooManyToRank reports whether more events match than
// max_filtered_events: ranking reads each one's vectors, and that is what
// kept memory growing with the history ("mensagens que recebi", no
// period). Zero means no limit.
func (a *Answerer) tooManyToRank(ctx context.Context, filter storage.EventFilter) (bool, error) {
	limit := a.settings.MaxFilteredEvents
	if limit <= 0 {
		return false, nil
	}
	count, err := a.store.CountMatching(ctx, filter, limit+1)
	return count > limit, err
}

// maxNeighbours is sqlite-vec's largest k.
const maxNeighbours = 4096

// searchAmong ranks a large filtered set through the vector index: the k
// nearest of the period and source, keeping those that satisfy the
// filter, widening k until top_k usable events remain. Neighbours beyond
// maxNeighbours are not seen; a filter that rare has few enough events
// to take the exact path.
func (a *Answerer) searchAmong(ctx context.Context, embedding []float32, query queryplan.Query, filter storage.EventFilter) ([]storage.ScoredEvent, error) {
	search := storage.SimilarityQuery{Embedding: embedding, Source: filter.Source, From: filter.From, To: filter.To, Among: &filter}
	for search.Limit = min(a.settings.TopK*chatterHeadroom, maxNeighbours); ; search.Limit = min(search.Limit*4, maxNeighbours) {
		hits, err := a.store.SearchSimilar(ctx, search)
		if err != nil {
			return nil, err
		}
		distinct := usableEvidence(bestChunkPerEvent(hits), query)
		if len(distinct) >= a.settings.TopK || search.Limit >= maxNeighbours {
			return distinct[:min(len(distinct), a.settings.TopK)], nil
		}
	}
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
