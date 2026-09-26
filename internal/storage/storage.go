// Package storage is the persistence boundary (RNF2.2). Ingestion, timeline
// and semantic search depend only on EventStore, so the SQLite backend can be
// swapped (e.g. for PostgreSQL/pgvector) without changing them.
package storage

import (
	"context"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// EventStore persists events and their embeddings.
type EventStore interface {
	// StoredEvent returns the stored event with this UID, if any, so
	// ingestion can skip unchanged events (and their embedding) on re-runs
	// and replace changed ones.
	StoredEvent(ctx context.Context, uid string) (event.Event, bool, error)
	// UpdateEvent replaces a stored event's fields and embedding (nil
	// removes it), keyed by UID; used when the source's version changed.
	UpdateEvent(ctx context.Context, ev event.Event, embedding []float32) error
	// SaveEvent stores the event and, when non-nil, its embedding. It returns
	// false without error when the UID already exists (RF1.5).
	SaveEvent(ctx context.Context, ev event.Event, embedding []float32) (bool, error)
	// EventsBetween returns events with from <= timestamp < to, oldest first.
	EventsBetween(ctx context.Context, from, to time.Time) ([]event.Event, error)
	// SearchSimilar returns the nearest events to the query embedding that
	// also satisfy the query's source and time filters.
	SearchSimilar(ctx context.Context, query SimilarityQuery) ([]ScoredEvent, error)
	// EmbeddingsFor returns the stored embeddings of the given event UIDs;
	// events without one are absent from the map. Used to rank an exactly
	// filtered set of events by similarity.
	EmbeddingsFor(ctx context.Context, uids []string) (map[string][]float32, error)
	// DeleteSource removes every event (and embedding) of source, returning
	// how many were removed; used to re-ingest after a collector changes.
	DeleteSource(ctx context.Context, source event.Source) (int, error)
	Close() error
}

// SimilarityQuery describes a filtered nearest-neighbour search.
type SimilarityQuery struct {
	Embedding []float32
	Limit     int
	// Source restricts results to one source; empty means all sources.
	Source event.Source
	// From and To bound the timestamp as From <= t < To; zero means unbounded.
	From time.Time
	To   time.Time
}

// ScoredEvent is a search hit. Distance is cosine distance: 0 is identical,
// 2 is opposite.
type ScoredEvent struct {
	Event    event.Event
	Distance float64
}

// EmbeddingIndex manages the vectors as a whole: which model produced them
// and rebuilding them with another (`cade reindex`). Vectors from two
// models are not comparable, even at the same dimension, so the model is
// recorded and a rebuild marks itself pending until it completes.
type EmbeddingIndex interface {
	// EmbeddingModel returns the recorded model name, "" if none yet.
	EmbeddingModel(ctx context.Context) (string, error)
	RecordEmbeddingModel(ctx context.Context, model string) error
	// StartReindex drops every vector, records model and marks a rebuild
	// pending.
	StartReindex(ctx context.Context, model string) error
	ReindexPending(ctx context.Context) (bool, error)
	// EventsWithoutEmbedding returns up to limit events with text and no
	// vector, in storage order.
	EventsWithoutEmbedding(ctx context.Context, limit int) ([]event.Event, error)
	CountEventsWithoutEmbedding(ctx context.Context) (int, error)
	SaveEmbeddings(ctx context.Context, embeddings []EventEmbedding) error
	FinishReindex(ctx context.Context) error
}

// EventEmbedding pairs a stored event with its new vector.
type EventEmbedding struct {
	Event  event.Event
	Vector []float32
}
