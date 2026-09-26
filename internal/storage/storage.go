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
	// StoredChunksForContent returns the embedded chunks of a stored event
	// with exactly this text, so identical text is embedded once.
	StoredChunksForContent(ctx context.Context, content string) ([]Chunk, bool, error)
	// UpdateEvent replaces a stored event's fields and chunks (none leaves
	// it without vectors), keyed by UID; used when the source's version
	// changed.
	UpdateEvent(ctx context.Context, ev event.Event, chunks []Chunk) error
	// SaveEvent stores the event and its embedded chunks. It returns false
	// without error when the UID already exists (RF1.5).
	SaveEvent(ctx context.Context, ev event.Event, chunks []Chunk) (bool, error)
	// EventsBetween returns events with from <= timestamp < to, oldest first.
	EventsBetween(ctx context.Context, from, to time.Time) ([]event.Event, error)
	// SearchSimilar returns the nearest chunks to the query embedding that
	// also satisfy the query's source and time filters, one hit per chunk
	// (an event can appear more than once), closest first.
	SearchSimilar(ctx context.Context, query SimilarityQuery) ([]ScoredEvent, error)
	// ChunksFor returns the embedded chunks of the given event UIDs, with
	// vectors; events without any are absent from the map. Used to rank an
	// exactly filtered set of events by similarity.
	ChunksFor(ctx context.Context, uids []string) (map[string][]Chunk, error)
	// FileModificationsBetween returns the file versions seen in
	// [from, to), oldest first: a file is one event, its edits live here.
	FileModificationsBetween(ctx context.Context, from, to time.Time) ([]FileModification, error)
	// MarkMissingFiles flags the file events under root whose path is not
	// in present as removed, and unflags those back; returns how many
	// were newly flagged.
	MarkMissingFiles(ctx context.Context, root string, present map[string]bool, at time.Time) (int, error)
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

// FileModification is one version of a file: when it was saved and its
// size.
type FileModification struct {
	Path       string
	ModifiedAt time.Time
	Size       int64
}

// Chunk is one embedded piece of an event's text: bytes [Start, End) of
// Content, the Ordinal-th (from 0) of the event's chunks. Short text is a
// single chunk covering it all.
type Chunk struct {
	Ordinal int
	Start   int
	End     int
	Vector  []float32
}

// Text is the chunk's part of content.
func (c Chunk) Text(content string) string {
	return content[min(c.Start, len(content)):min(c.End, len(content))]
}

// ScoredEvent is a search hit. Distance is cosine distance: 0 is identical,
// 2 is opposite. Chunk is the piece that matched (without its vector) and
// ChunkCount how many the event has.
type ScoredEvent struct {
	Event      event.Event
	Distance   float64
	Chunk      Chunk
	ChunkCount int
	// Repeats counts other events retrieval folded into this one (more
	// visits to the page, older versions of the file); LatestAt is the
	// most recent of them all. Zero when nothing was folded.
	Repeats  int
	LatestAt time.Time
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

// EventEmbedding pairs a stored event with its new embedded chunks.
type EventEmbedding struct {
	Event  event.Event
	Chunks []Chunk
}
