// Package testfakes holds named in-memory stand-ins for the storage and
// model boundaries, shared by tests across packages.
package testfakes

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// FakeEventStore is an in-memory storage.EventStore. SearchSimilar returns
// the canned SearchResults (filtered like the real store) and records the
// last query.
type FakeEventStore struct {
	Events     []event.Event
	Embeddings map[string][]float32
	// Chunks holds the chunks given to SaveEvent, UpdateEvent and
	// SaveEmbeddings; Embeddings mirrors each event's first chunk vector.
	Chunks        map[string][]storage.Chunk
	SearchResults []storage.ScoredEvent
	// Updated lists the UIDs passed to UpdateEvent, in order.
	Updated   []string
	LastQuery storage.SimilarityQuery
	FailWith  error
	Closed    bool
	Forgotten map[string]bool
	// Rekeyed is the last change set given to RekeyEvents.
	Rekeyed map[string]string
	// EmbeddingModelName, Pending and ReindexStarted back the
	// storage.EmbeddingIndex methods.
	EmbeddingModelName string
	Calibration        storage.ThresholdCalibration
	Pending            bool
	ReindexStarted     int
	// Modifications backs FileModificationsBetween; MarkMissingFiles
	// records its roots and paths and reports MissingFiles as removed.
	Modifications []storage.FileModification
	MarkedRoots   []string
	LastPresent   map[string]bool
	MissingFiles  int
	// LexicalResults are returned by SearchLexical, which records each
	// query in LexicalQueries.
	LexicalResults []storage.ScoredEvent
	LexicalQueries []storage.LexicalQuery
	// AuthorshipMarks lists the repositories passed to MarkCommitAuthorship.
	AuthorshipMarks []string
	// BatchesBegun, Commits and Rollbacks count FakeEventBatch calls.
	BatchesBegun int
	Commits      int
	Rollbacks    int
	// Slots backs VectorSlots; CompactVectors counts itself in Compactions
	// and replaces Slots with CompactedSlots.
	Slots          storage.VectorSlots
	CompactedSlots storage.VectorSlots
	Compactions    int
}

// NewFakeEventStore returns an empty store.
func NewFakeEventStore() *FakeEventStore {
	return &FakeEventStore{Embeddings: map[string][]float32{}}
}

func (f *FakeEventStore) StoredEvent(_ context.Context, uid string) (event.Event, bool, error) {
	index := f.indexOf(uid)
	if index < 0 {
		return event.Event{}, false, f.FailWith
	}
	return f.Events[index], true, f.FailWith
}

func (f *FakeEventStore) indexOf(uid string) int {
	for i, ev := range f.Events {
		if ev.UID == uid {
			return i
		}
	}
	return -1
}

// UpdateEvent replaces the event and records the UID in Updated.
func (f *FakeEventStore) UpdateEvent(_ context.Context, ev event.Event, chunks []storage.Chunk) error {
	index := f.indexOf(ev.UID)
	if index < 0 || f.FailWith != nil {
		return f.FailWith
	}
	f.Events[index] = ev
	f.storeChunks(ev.UID, chunks)
	f.Updated = append(f.Updated, ev.UID)
	return nil
}

func (f *FakeEventStore) SaveEvent(_ context.Context, ev event.Event, chunks []storage.Chunk) (bool, error) {
	if f.indexOf(ev.UID) >= 0 || f.FailWith != nil {
		return false, f.FailWith
	}
	f.Events = append(f.Events, ev)
	f.storeChunks(ev.UID, chunks)
	return true, nil
}

func (f *FakeEventStore) EventsBetween(_ context.Context, from, to time.Time) ([]event.Event, error) {
	var matching []event.Event
	for _, ev := range f.Events {
		if !ev.Timestamp.Before(from) && ev.Timestamp.Before(to) {
			matching = append(matching, ev)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool { return matching[i].Timestamp.Before(matching[j].Timestamp) })
	return matching, f.FailWith
}

func (f *FakeEventStore) SearchSimilar(_ context.Context, query storage.SimilarityQuery) ([]storage.ScoredEvent, error) {
	f.LastQuery = query
	var hits []storage.ScoredEvent
	for _, hit := range f.SearchResults {
		if len(hits) < query.Limit && (query.Source == "" || hit.Event.Source == query.Source) {
			hits = append(hits, hit)
		}
	}
	return f.keepAmong(hits, query.Among), f.FailWith
}

func (f *FakeEventStore) DeleteSource(_ context.Context, source event.Source) (int, error) {
	var kept []event.Event
	for _, ev := range f.Events {
		if ev.Source != source {
			kept = append(kept, ev)
		}
	}
	removed := len(f.Events) - len(kept)
	f.Events = kept
	f.Forgotten = map[string]bool{}
	return removed, f.FailWith
}

func (f *FakeEventStore) DeleteEvent(_ context.Context, uid string) (bool, error) {
	index := f.indexOf(uid)
	if index < 0 {
		return false, f.FailWith
	}
	f.Events = append(f.Events[:index], f.Events[index+1:]...)
	if f.Forgotten == nil {
		f.Forgotten = map[string]bool{}
	}
	f.Forgotten[uid] = true
	return true, f.FailWith
}

// RekeyEvents renames stored UIDs and carries forgotten marks, like the
// real store, and records the changes it was given.
func (f *FakeEventStore) RekeyEvents(_ context.Context, changes map[string]string) (storage.RekeyReport, error) {
	f.Rekeyed = changes
	report := storage.RekeyReport{BackupPath: "/data/cade.db.before-rekey"}
	for oldUID, newUID := range changes {
		if f.Forgotten[oldUID] {
			f.Forgotten[newUID] = true
		}
		if index := f.indexOf(oldUID); index >= 0 && f.indexOf(newUID) < 0 {
			f.Events[index].UID = newUID
			report.Rekeyed++
		}
	}
	return report, f.FailWith
}

func (f *FakeEventStore) IsForgotten(_ context.Context, uid string) (bool, error) {
	return f.Forgotten[uid], f.FailWith
}

func (f *FakeEventStore) EventsContaining(_ context.Context, text string, filter storage.EventFilter) ([]event.Event, error) {
	var result []event.Event
	for _, ev := range f.Events {
		if (filter.Source == "" || ev.Source == filter.Source) && strings.Contains(strings.ToLower(ev.Content), strings.ToLower(text)) {
			result = append(result, ev)
		}
	}
	return result, f.FailWith
}

func (f *FakeEventStore) DeleteBefore(_ context.Context, source event.Source, before time.Time) (int, error) {
	removed := 0
	kept := f.Events[:0]
	for _, ev := range f.Events {
		if ev.Source == source && ev.Timestamp.Before(before) {
			if f.Forgotten == nil {
				f.Forgotten = map[string]bool{}
			}
			f.Forgotten[ev.UID] = true
			removed++
			continue
		}
		kept = append(kept, ev)
	}
	f.Events = kept
	return removed, f.FailWith
}

func (f *FakeEventStore) Close() error {
	f.Closed = true
	return nil
}

// ChunksFor returns the stored chunks; an event given only a vector in
// Embeddings is one chunk covering its whole text.
func (f *FakeEventStore) ChunksFor(_ context.Context, uids []string) (map[string][]storage.Chunk, error) {
	found := map[string][]storage.Chunk{}
	for _, uid := range uids {
		if chunks := f.chunksOf(uid); len(chunks) > 0 {
			found[uid] = chunks
		}
	}
	return found, f.FailWith
}

func (f *FakeEventStore) chunksOf(uid string) []storage.Chunk {
	if chunks := f.Chunks[uid]; len(chunks) > 0 {
		return chunks
	}
	vector := f.Embeddings[uid]
	if vector == nil {
		return nil
	}
	end := 0
	if index := f.indexOf(uid); index >= 0 {
		end = len(f.Events[index].Content)
	}
	return []storage.Chunk{{End: end, Vector: vector}}
}

// storeChunks keeps the chunks and, as a shortcut for tests, the first
// chunk's vector in Embeddings.
func (f *FakeEventStore) storeChunks(uid string, chunks []storage.Chunk) {
	if f.Chunks == nil {
		f.Chunks = map[string][]storage.Chunk{}
	}
	f.Chunks[uid], f.Embeddings[uid] = chunks, nil
	if len(chunks) > 0 {
		f.Embeddings[uid] = chunks[0].Vector
	}
}

// Embedding index: the fake records the model and a pending rebuild, and
// treats events with text and no entry in Embeddings as missing a vector.

func (f *FakeEventStore) EmbeddingModel(context.Context) (string, error) {
	return f.EmbeddingModelName, f.FailWith
}

func (f *FakeEventStore) ThresholdCalibration(context.Context) (storage.ThresholdCalibration, error) {
	return f.Calibration, f.FailWith
}

func (f *FakeEventStore) RecordThresholdCalibration(_ context.Context, calibration storage.ThresholdCalibration) error {
	f.Calibration = calibration
	return f.FailWith
}

func (f *FakeEventStore) RecordEmbeddingModel(_ context.Context, model string) error {
	f.EmbeddingModelName = model
	return f.FailWith
}

func (f *FakeEventStore) StartReindex(_ context.Context, model string) error {
	f.Embeddings, f.Chunks, f.EmbeddingModelName, f.ReindexStarted, f.Pending = map[string][]float32{}, nil, model, f.ReindexStarted+1, true
	return f.FailWith
}

func (f *FakeEventStore) ReindexPending(context.Context) (bool, error) {
	return f.Pending, f.FailWith
}

func (f *FakeEventStore) EventsWithoutEmbedding(_ context.Context, limit int) ([]event.Event, error) {
	missing := f.missingEmbeddings()
	return missing[:min(limit, len(missing))], f.FailWith
}

func (f *FakeEventStore) CountEventsWithoutEmbedding(context.Context) (int, error) {
	return len(f.missingEmbeddings()), f.FailWith
}

func (f *FakeEventStore) missingEmbeddings() []event.Event {
	var missing []event.Event
	for _, ev := range f.Events {
		if ev.Content != "" && len(f.chunksOf(ev.UID)) == 0 {
			missing = append(missing, ev)
		}
	}
	return missing
}

func (f *FakeEventStore) SaveEmbeddings(_ context.Context, embeddings []storage.EventEmbedding) error {
	if f.FailWith != nil {
		return f.FailWith
	}
	for _, pair := range embeddings {
		f.storeChunks(pair.Event.UID, pair.Chunks)
	}
	return nil
}

func (f *FakeEventStore) FinishReindex(context.Context) error {
	f.Pending = false
	return f.FailWith
}

// StoredChunksForContent returns the chunks of an event with the same
// content, as the real store does by content hash.
func (f *FakeEventStore) StoredChunksForContent(_ context.Context, content string) ([]storage.Chunk, bool, error) {
	for _, ev := range f.Events {
		if chunks := f.chunksOf(ev.UID); ev.Content == content && len(chunks) > 0 {
			return chunks, true, f.FailWith
		}
	}
	return nil, false, f.FailWith
}

// FileModificationsBetween returns Modifications within [from, to).
func (f *FakeEventStore) FileModificationsBetween(_ context.Context, from, to time.Time) ([]storage.FileModification, error) {
	var kept []storage.FileModification
	for _, modification := range f.Modifications {
		if !modification.ModifiedAt.Before(from) && modification.ModifiedAt.Before(to) {
			kept = append(kept, modification)
		}
	}
	return kept, f.FailWith
}

// MarkMissingFiles records the call and reports MissingFiles as removed.
func (f *FakeEventStore) MarkMissingFiles(_ context.Context, root string, present map[string]bool, _ time.Time) (int, error) {
	f.MarkedRoots, f.LastPresent = append(f.MarkedRoots, root), present
	return f.MissingFiles, f.FailWith
}

// SearchLexical records the query and returns LexicalResults filtered like
// the real store.
func (f *FakeEventStore) SearchLexical(_ context.Context, query storage.LexicalQuery) ([]storage.ScoredEvent, error) {
	f.LexicalQueries = append(f.LexicalQueries, query)
	var hits []storage.ScoredEvent
	for _, hit := range f.LexicalResults {
		if len(hits) < query.Limit && (query.Source == "" || hit.Event.Source == query.Source) {
			hits = append(hits, hit)
		}
	}
	return hits, f.FailWith
}

// MarkCommitAuthorship marks the fake's commits of repository like the
// real store and records the call.
func (f *FakeEventStore) MarkCommitAuthorship(_ context.Context, repository string, identities []string) (int, error) {
	f.AuthorshipMarks = append(f.AuthorshipMarks, repository)
	changed := 0
	for i, ev := range f.Events {
		commit := ev.Commit()
		if ev.Source != event.SourceGit || commit.Repository != repository || len(identities) == 0 {
			continue
		}
		commit.Authorship = event.AuthorshipOther
		if slices.Contains(identities, strings.ToLower(commit.Email)) || slices.Contains(identities, strings.ToLower(commit.Author)) {
			commit.Authorship = event.AuthorshipMine
		}
		f.Events[i].Metadata["authorship"], changed = string(commit.Authorship), changed+1
	}
	return changed, f.FailWith
}

var _ storage.ImageCaptionIndex = (*FakeEventStore)(nil)

// DescribedImage returns the image of the first described event with this
// hash.
func (f *FakeEventStore) DescribedImage(_ context.Context, sha256 string) (event.Image, bool, error) {
	for _, ev := range f.Events {
		if image := ev.Image(); image.SHA256 == sha256 && image.Status == event.CaptionDescribed {
			return image, true, f.FailWith
		}
	}
	return event.Image{}, false, f.FailWith
}

// OutdatedImages lists the described, present image events whose model or
// prompt version differs.
func (f *FakeEventStore) OutdatedImages(_ context.Context, model string, promptVersion int) ([]event.Event, error) {
	var outdated []event.Event
	for _, ev := range f.Events {
		image := ev.Image()
		present := ev.File().RemovedAt.IsZero()
		if image.Status == event.CaptionDescribed && present && (image.Model != model || image.PromptVersion != promptVersion) {
			outdated = append(outdated, ev)
		}
	}
	return outdated, f.FailWith
}

func (f *FakeEventStore) VectorSlots(context.Context) (storage.VectorSlots, error) {
	return f.Slots, f.FailWith
}

func (f *FakeEventStore) CompactVectors(context.Context) (storage.VectorSlots, error) {
	if f.FailWith != nil {
		return storage.VectorSlots{}, f.FailWith
	}
	f.Compactions++
	f.Slots = f.CompactedSlots
	return f.Slots, nil
}
