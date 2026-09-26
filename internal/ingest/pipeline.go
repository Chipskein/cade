// Package ingest turns source-specific collectors into stored, embedded
// events. A new source only needs an EventCollector (RNF4.1).
package ingest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
)

// maxEmbeddedChars caps the text sent to the embedder; the model truncates
// to its context anyway, this just avoids tokenizing huge files.
const maxEmbeddedChars = 8000

// EmitFunc receives each event a collector produces. Returning an error
// stops the collection.
type EmitFunc func(event.Event) error

// EventCollector reads one source target and emits normalized events.
type EventCollector interface {
	CollectEvents(ctx context.Context, emit EmitFunc) error
}

// Report counts what one ingestion run did.
type Report struct {
	Collected int
	Inserted  int
	// Updated counts stored events whose content changed at the source
	// (an edited Teams message) and were replaced.
	Updated       int
	AlreadyStored int
}

// Pipeline embeds and stores collected events, skipping known ones (RF1.5).
type Pipeline struct {
	store          storage.EventStore
	embedder       llm.Embedder
	documentPrefix string
	logger         *slog.Logger
}

// NewPipeline wires a pipeline. documentPrefix is prepended to the text
// before embedding (e.g. "search_document: ").
//
//	pipeline := ingest.NewPipeline(store, embedder, "search_document: ", logger)
func NewPipeline(store storage.EventStore, embedder llm.Embedder, documentPrefix string, logger *slog.Logger) *Pipeline {
	return &Pipeline{store: store, embedder: embedder, documentPrefix: documentPrefix, logger: logger}
}

// Run drains the collector. Any failure aborts the run rather than skipping
// the event, because a silently missing event breaks timeline trust
// (RNF3.1); re-running is safe thanks to deduplication.
// progress, when non-nil, receives the running totals after every event.
func (p *Pipeline) Run(ctx context.Context, collector EventCollector, progress ProgressFunc) (Report, error) {
	if progress == nil {
		progress = func(Report) {}
	}
	var report Report
	err := collector.CollectEvents(ctx, func(ev event.Event) error {
		report.Collected++
		err := p.ingestEvent(ctx, ev, &report)
		progress(report)
		return err
	})
	return report, err
}

// ProgressFunc observes a run's running totals.
type ProgressFunc func(Report)

func (p *Pipeline) ingestEvent(ctx context.Context, ev event.Event, report *Report) error {
	stored, known, err := p.store.StoredEvent(ctx, ev.UID)
	if err != nil {
		return err
	}
	if known && !replaces(ev, stored) {
		report.AlreadyStored++
		return nil
	}
	embedding, err := p.embeddingFor(ev)
	if err != nil {
		return err
	}
	if known {
		return p.update(ctx, ev, embedding, report)
	}
	return p.insert(ctx, ev, embedding, report)
}

// replaces reports whether incoming should overwrite stored: its content
// changed and, when both carry a revision, it is newer. Equal revisions
// keep the stored event, so two Teams caches holding different renderings
// of one version do not alternate on every run.
func replaces(incoming, stored event.Event) bool {
	if incoming.Content == stored.Content {
		return false
	}
	incomingRevision, hasIncoming := incoming.Revision()
	storedRevision, hasStored := stored.Revision()
	return !hasIncoming || !hasStored || incomingRevision > storedRevision
}

func (p *Pipeline) insert(ctx context.Context, ev event.Event, embedding []float32, report *Report) error {
	inserted, err := p.store.SaveEvent(ctx, ev, embedding)
	if err != nil {
		return err
	}
	report.Inserted += boolToInt(inserted)
	report.AlreadyStored += boolToInt(!inserted)
	p.logger.Debug("event ingested", "uid", ev.UID, "source", ev.Source, "inserted", inserted)
	return nil
}

func (p *Pipeline) update(ctx context.Context, ev event.Event, embedding []float32, report *Report) error {
	if err := p.store.UpdateEvent(ctx, ev, embedding); err != nil {
		return err
	}
	report.Updated++
	p.logger.Debug("event updated", "uid", ev.UID, "source", ev.Source)
	return nil
}

// embeddingFor returns nil for events without text: they still appear in
// the timeline, they just cannot be found by semantic search (RF2.2).
func (p *Pipeline) embeddingFor(ev event.Event) ([]float32, error) {
	if ev.Content == "" {
		return nil, nil
	}
	text := truncateRunes(ev.Content, maxEmbeddedChars)
	embedding, err := p.embedder.Embed(p.documentPrefix + text)
	if err != nil {
		return nil, fmt.Errorf("embed %s event %q: %w", ev.Source, ev.UID, err)
	}
	return embedding, nil
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
