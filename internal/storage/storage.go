// Package storage is the persistence boundary (RNF2.2). Ingestion, timeline
// and semantic search depend only on EventStore, so the SQLite backend can be
// swapped (e.g. for PostgreSQL/pgvector) without changing them.
package storage

import (
	"context"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
)

// EventWriter is what ingestion reads and writes for each collected event.
type EventWriter interface {
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
	// IsForgotten reports a UID removed by `cade forget` or retention,
	// which ingestion must not store again.
	IsForgotten(ctx context.Context, uid string) (bool, error)
}

// EventBatch is an EventWriter whose writes land in one transaction
// (issue #51): a commit per event rewrote every changed page and synced,
// writing ~28× the database's size on a first ingestion. While a batch is
// open, use only the batch: the store may have no other connection.
type EventBatch interface {
	EventWriter
	// Commit makes the batch's writes durable. After any error from the
	// batch, Rollback instead: the failed event may be half written.
	Commit() error
	// Rollback discards the batch; it is safe after Commit.
	Rollback() error
}

// EventStore persists events and their embeddings.
type EventStore interface {
	EventWriter
	// BeginBatch opens a batch; the caller must Commit or Rollback it
	// before using the store again.
	BeginBatch(ctx context.Context) (EventBatch, error)
	// EventsBetween returns events with from <= timestamp < to, oldest first.
	EventsBetween(ctx context.Context, from, to time.Time) ([]event.Event, error)
	// SearchSimilar returns the nearest chunks to the query embedding that
	// also satisfy the query's source and time filters, one hit per chunk
	// (an event can appear more than once), closest first.
	SearchSimilar(ctx context.Context, query SimilarityQuery) ([]ScoredEvent, error)
	// SearchLexical returns the chunks matching a keyword query that also
	// satisfy its source and time filters, best match first; hits carry
	// no distance.
	SearchLexical(ctx context.Context, query LexicalQuery) ([]ScoredEvent, error)
	// CountMatching counts the events satisfying filter, stopping at upTo;
	// it reads no content, so it is cheap on the whole history.
	CountMatching(ctx context.Context, filter EventFilter, upTo int) (int, error)
	// EventsMatching returns the events satisfying filter, oldest first.
	EventsMatching(ctx context.Context, filter EventFilter) ([]event.Event, error)
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
	// MarkCommitAuthorship marks the stored commits of repository as the
	// user's (author email or name in identities) or someone else's;
	// returns how many marks changed.
	MarkCommitAuthorship(ctx context.Context, repository string, identities []string) (int, error)
	// DeleteSource removes every event (and embedding) of source, returning
	// how many were removed; used to re-ingest after a collector changes.
	DeleteSource(ctx context.Context, source event.Source) (int, error)
	DeleteEvent(ctx context.Context, uid string) (bool, error)
	EventsContaining(ctx context.Context, text string, filter EventFilter) ([]event.Event, error)
	DeleteBefore(ctx context.Context, source event.Source, before time.Time) (int, error)
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
	// Among, when set, keeps only hits satisfying it. It applies after the
	// k nearest are found: k counts every chunk of the source and period.
	Among *EventFilter
}

// EventFilter selects events exactly, as listing.Select does in memory:
// timestamp in [From, To), the source (empty means all), the message
// direction, any of People (everyone when empty), and files under Folder
// (anywhere when empty).
type EventFilter struct {
	From      time.Time
	To        time.Time
	Source    event.Source
	Direction listing.Direction
	People    []listing.PersonMatcher
	Folder    string
}

// LexicalQuery describes a filtered keyword search. Match is an FTS5
// expression over folded words ("\"redis\" OR \"cache\"", "\"e5f6a7b\"*").
type LexicalQuery struct {
	Match  string
	Limit  int
	Source event.Source
	From   time.Time
	To     time.Time
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
	// ThresholdCalibration returns the recorded gates, zero if none yet.
	ThresholdCalibration(ctx context.Context) (ThresholdCalibration, error)
	RecordThresholdCalibration(ctx context.Context, calibration ThresholdCalibration) error
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

// DatabaseState is what `cade doctor` reads from the database without
// changing it: opening a store would apply migrations and may copy it.
type DatabaseState struct {
	// Exists is false before the first command creates the database.
	Exists bool
	// FTS5 tells whether this binary's SQLite can build the keyword index.
	FTS5 bool
	// SchemaVersion is the database's; LatestSchemaVersion this binary's.
	SchemaVersion       int
	LatestSchemaVersion int
	// ThresholdCalibration is the model the retrieval gates were set for.
	ThresholdCalibration ThresholdCalibration
	// MigrationBackup: a pending migration copies the database (SizeBytes
	// more on disk) before rewriting it.
	MigrationBackup bool
	SizeBytes       int64
	EmbeddingModel  string
	ReindexPending  bool
}

// ThresholdCalibration pairs the retrieval distance gates with the
// embedding model they were set for: distances of another model live on
// another scale, and gates left over from it answer "not found" to
// questions that have an answer (phase 17).
type ThresholdCalibration struct {
	Model           string
	MaxDistance     float64
	MaxBestDistance float64
}

// OrIndexedWith fills a missing record: vectors that predate it were
// searched with the current gates, set for the vectors' model.
//
//	recorded.OrIndexedWith(state.EmbeddingModel, current).OutdatedFor(current)
func (c ThresholdCalibration) OrIndexedWith(indexedModel string, current ThresholdCalibration) ThresholdCalibration {
	if c.Model != "" {
		return c
	}
	return ThresholdCalibration{Model: indexedModel, MaxDistance: current.MaxDistance, MaxBestDistance: current.MaxBestDistance}
}

// OutdatedFor reports gates set for another model and unchanged since:
// changing either gate counts as recalibrating for current.Model.
func (c ThresholdCalibration) OutdatedFor(current ThresholdCalibration) bool {
	sameGates := c.MaxDistance == current.MaxDistance && c.MaxBestDistance == current.MaxBestDistance
	return c.Model != "" && c.Model != current.Model && sameGates
}

// ImageCaptionIndex finds a description by the image's content (phase 19):
// a moved, renamed or copied image is not described again.
type ImageCaptionIndex interface {
	// DescribedImage returns the image metadata of a described file event
	// whose image has this SHA-256, if any; a file gone from its folder
	// still counts, since the image may have moved.
	DescribedImage(ctx context.Context, sha256 string) (event.Image, bool, error)
	// OutdatedImages returns the described image events still in their
	// folder whose description another model or prompt version wrote, for
	// `cade reindex --captions`.
	OutdatedImages(ctx context.Context, model string, promptVersion int) ([]event.Event, error)
}
