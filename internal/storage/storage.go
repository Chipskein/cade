// Package storage is the persistence boundary (RNF2.2). Ingestion, timeline
// and semantic search depend only on EventStore, so the SQLite backend can be
// swapped (e.g. for PostgreSQL/pgvector) without changing them.
package storage

import (
	"context"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/representation"
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

// RekeyReport is what RekeyEvents changed, and where the copy taken
// before it is.
type RekeyReport struct {
	Rekeyed    int
	Conflicts  int
	BackupPath string
}

// EventStore persists events and their embeddings.
type EventStore interface {
	EventWriter
	// BeginBatch opens a batch; the caller must Commit or Rollback it
	// before using the store again.
	BeginBatch(ctx context.Context) (EventBatch, error)
	// EventsBetween returns events with from <= timestamp < to, oldest first.
	EventsBetween(ctx context.Context, from, to time.Time) ([]event.Event, error)
	// SearchSimilar returns the query.Limit nearest chunks to the query
	// embedding that have an event in the query's source and period, one
	// hit per such event (a text's events share its chunks, and an event
	// can appear once per chunk), closest first.
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
	// RekeyEvents gives stored events new UIDs (old to new), keeping their
	// content, vectors and forgotten marks; an event whose new UID is
	// already stored is left as is and counted as a conflict. The
	// database is copied first.
	RekeyEvents(ctx context.Context, changes map[string]string) (RekeyReport, error)
	EventsContaining(ctx context.Context, text string, filter EventFilter) ([]event.Event, error)
	DeleteBefore(ctx context.Context, source event.Source, before time.Time) (int, error)
	VectorCompactor
	// EntityIndex and RelationIndex let retrieval widen a question
	// through the graph (#24).
	EntityIndex
	RelationIndex
	// EventPager lets `cade export` walk the whole history (#85).
	EventPager
	Close() error
}

// VectorCompactor returns to the disk the vector positions deletions left
// empty (`cade compact`, #65).
type VectorCompactor interface {
	// VectorSlots measures the blocks; zero before the first vector.
	VectorSlots(ctx context.Context) (VectorSlots, error)
	// CompactVectors rewrites the blocks with only the live vectors and
	// returns them as they are afterwards; the vectors stay the same.
	CompactVectors(ctx context.Context) (VectorSlots, error)
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
	// k nearest are found: k counts every chunk with an event in the
	// source and period.
	Among *EventFilter
}

// EventFilter selects events exactly, as listing.Select does in memory:
// timestamp in [From, To), the source (empty means all), the message
// direction, any of People (everyone when empty), files under Folder
// (anywhere when empty), and Graph.
type EventFilter struct {
	From      time.Time
	To        time.Time
	Source    event.Source
	Direction listing.Direction
	People    []listing.PersonMatcher
	Folder    string
	Graph     GraphFilter
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
	// VectorSlots measures the vector blocks, for `cade compact`.
	VectorSlots VectorSlots
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

// VectorSlots measures the vector blocks: Slots is every position they
// hold, Vectors the live ones. sqlite-vec never reuses a deleted position
// and VACUUM cannot reach inside its blocks (#65), so the difference is
// space the search still reads until the vectors are compacted.
type VectorSlots struct {
	Slots   int
	Vectors int
}

// EmptyShare is the fraction of positions without a vector, 0 when the
// blocks are full or there are none.
//
//	storage.VectorSlots{Slots: 2048, Vectors: 1024}.EmptyShare() // 0.5
func (v VectorSlots) EmptyShare() float64 {
	if v.Slots == 0 {
		return 0
	}
	return float64(v.Slots-v.Vectors) / float64(v.Slots)
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

// Entity is a stored entity (phase 26, #20): what several events refer
// to. ID is stable while some event refers to it; Name is the first
// spelling stored.
type Entity struct {
	ID   int64
	Type entity.Type
	Key  string
	Name string
}

// EntityLink is an entity as one event refers to it.
type EntityLink struct {
	Entity
	Role entity.Role
}

// EntityIndex reads the entities the store derived from the events, the
// base for relations (#21) and for a person's export and forget (#85, #86).
type EntityIndex interface {
	// EntitiesOfEvent lists what the event uid refers to, by type, key and
	// role; none for an unknown uid.
	EntitiesOfEvent(ctx context.Context, uid string) ([]EntityLink, error)
	// EventsOfEntity lists the UIDs of the events referring to the entity,
	// oldest first; none for an unknown id.
	EventsOfEntity(ctx context.Context, entityID int64) ([]string, error)
}

// MaxRelationHops caps how far Reachable walks: past three hops, almost
// everything in a history is connected through some shared file or person.
const MaxRelationHops = 3

// Relationship is a stored relation between two entities (#21), with the
// event that states it.
type Relationship struct {
	From     Entity
	Relation entity.Relation
	To       Entity
	// Confidence is nil when the source states the relation, and set only
	// for one inferred automatically.
	Confidence     *float64
	SourceEventUID string
}

// ReachedEntity is an entity Reachable found and the fewest hops to it.
type ReachedEntity struct {
	Entity
	Hops int
}

// RelationIndex walks the relationships between entities. An empty
// relation means any.
type RelationIndex interface {
	// RelationsFrom lists the relations whose origin is entityID.
	RelationsFrom(ctx context.Context, entityID int64, relation entity.Relation) ([]Relationship, error)
	// RelationsTo lists the relations whose target is entityID.
	RelationsTo(ctx context.Context, entityID int64, relation entity.Relation) ([]Relationship, error)
	// Reachable lists the entities within hops relations of entityID, in
	// either direction, nearest first; hops above MaxRelationHops are
	// capped, and the entity itself is left out.
	Reachable(ctx context.Context, entityID int64, hops int) ([]ReachedEntity, error)
	// RelationsStatedBy lists the relations the event uid states, for the
	// export (#85).
	RelationsStatedBy(ctx context.Context, uid string) ([]Relationship, error)
}

// EntityKey names an entity by what callers know of it: an image's
// SHA-256, a file's path.
type EntityKey struct {
	Type entity.Type
	Key  string
}

// StoredRepresentation is a representation as stored, with its id.
type StoredRepresentation struct {
	ID int64
	representation.Representation
}

// RepresentationIndex keeps what models derived from entities (#22).
type RepresentationIndex interface {
	// SaveRepresentation stores rep for the entity owner, replacing the
	// one of the same modality and kind; the entity must be stored.
	SaveRepresentation(ctx context.Context, owner EntityKey, rep representation.Representation) (int64, error)
	// RepresentationsOf lists the entity's representations by modality and
	// kind; none for an unknown entity.
	RepresentationsOf(ctx context.Context, owner EntityKey) ([]StoredRepresentation, error)
	// SaveRepresentationVector stores the vector of a stored
	// representation in the vector index; replacing the representation's
	// content drops it. Every vector has the dimension of the first.
	SaveRepresentationVector(ctx context.Context, representationID int64, vector []float32) error
	// NearestRepresentations lists the representations of kind whose
	// vectors are closest to vector, closest first.
	NearestRepresentations(ctx context.Context, vector []float32, kind representation.Kind, limit int) ([]ScoredRepresentation, error)
}

// ScoredRepresentation is a representation found by its vector, with its
// entity and cosine distance.
type ScoredRepresentation struct {
	StoredRepresentation
	Owner    EntityKey
	Distance float64
}
