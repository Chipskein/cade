package retrievalsuite

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/ingest/pdfplan"
	"github.com/chipskein/cade/internal/pdffile"
)

// Generous limits: a fixture is small, and the suite measures reading, not them.
const (
	fixtureMaxPDFBytes = 1 << 26
	fixtureMaxPDFPages = 100
)

// noStoredPDFs is an empty history: to the suite, every fixture is new.
type noStoredPDFs struct{}

func (noStoredPDFs) IsForgotten(context.Context, string) (bool, error) {
	return false, nil
}

func (noStoredPDFs) StoredEvent(context.Context, string) (event.Event, bool, error) {
	return event.Event{}, false, nil
}

// ReadPDFs fills the text and metadata of the events naming a PDF fixture in
// directory, through the planner ingestion uses, so the suites search what a
// real run would store (#108). describer gives the scanned pages' descriptions.
//
//	err := corpus.ReadPDFs(ctx, "testdata/pdfs", captions)
func (c *Corpus) ReadPDFs(ctx context.Context, directory string, describer FixtureDescriber) error {
	readings, err := readFixturePDFs(ctx, directory, describer)
	if err != nil {
		return err
	}
	for i := range c.Events {
		if c.Events[i].PDF != "" {
			if err := fillPDFEvent(&c.Events[i], directory, readings); err != nil {
				return fmt.Errorf("corpus event %q: %w", c.Events[i].ID, err)
			}
		}
	}
	return nil
}

func readFixturePDFs(ctx context.Context, directory string, describer FixtureDescriber) (ingest.PDFReadings, error) {
	describe := func(ctx context.Context, key string, raw []byte) (event.Image, error) {
		return describer.Describe(ctx, key, raw)
	}
	settings := pdfplan.Settings{MaxBytes: fixtureMaxPDFBytes, MaxPages: fixtureMaxPDFPages}
	planner := pdfplan.NewPlanner(noStoredPDFs{}, pdffile.ExtractorReader{}, settings, describe, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err := planner.PlanDirectory(ctx, os.DirFS(directory), directory, filesource.Options{}); err != nil {
		return nil, fmt.Errorf("read the PDF fixtures of %q: %w", directory, err)
	}
	return planner.Readings(), nil
}

// fillPDFEvent places a reading as the file collector does: a file event's text
// is its name and its pages.
func fillPDFEvent(corpusEvent *CorpusEvent, directory string, readings ingest.PDFReadings) error {
	path := (event.Event{Metadata: corpusEvent.Metadata}).File().Path
	if corpusEvent.Source != event.SourceFile || path == "" {
		return fmt.Errorf("PDF %q on a %s event, expected a file event with metadata.path", corpusEvent.PDF, corpusEvent.Source)
	}
	reading, read := readings[event.FilePathUnder(directory, corpusEvent.PDF)]
	if !read || reading.PDF.Status == event.PDFUnreadable {
		return fmt.Errorf("PDF %q in %q was not read: %+v, expected a readable fixture", corpusEvent.PDF, directory, reading.PDF)
	}
	metadata := maps.Clone(corpusEvent.Metadata)
	maps.Copy(metadata, reading.PDF.Metadata())
	if len(reading.PageImages) > 0 {
		maps.Copy(metadata, event.PDFPageImagesMetadata(reading.PageImages))
	}
	corpusEvent.Content, corpusEvent.Metadata = filesource.EventContent(path, reading.Text), metadata
	return nil
}
