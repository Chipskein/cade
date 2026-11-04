// Package pdfplan reads the PDFs of a run before the file collector does
// (#108). The collector has no access to the store, and on its own it would
// parse every PDF again on every run.
package pdfplan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/chunking"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/pdffile"
)

// pageTimeout bounds reading one page; a real one takes about a millisecond.
// A page that hits it is a crafted file, so the whole file is given up on: 30 s
// per page of 500 would otherwise hold a run for hours.
const pageTimeout = 30 * time.Second

// Store is what planning reads from the database.
type Store interface {
	IsForgotten(ctx context.Context, uid string) (bool, error)
	StoredEvent(ctx context.Context, uid string) (event.Event, bool, error)
}

// Settings are the limits of one file.
type Settings struct {
	MaxBytes int64
	MaxPages int
}

// Tally counts what planning did with the PDFs of a run.
type Tally struct {
	Read       int
	Pending    int
	Unreadable int
}

func (t *Tally) add(status event.PDFStatus) {
	switch status {
	case event.PDFRead:
		t.Read++
	case event.PDFPending:
		t.Pending++
	case event.PDFUnreadable:
		t.Unreadable++
	}
}

// Planner decides, file by file, what the collector stores for each PDF.
type Planner struct {
	store    Store
	reader   pdffile.Reader
	settings Settings
	logger   *slog.Logger
	readings ingest.PDFReadings
	tally    Tally
}

// NewPlanner plans against store.
//
//	planner := pdfplan.NewPlanner(store, pdffile.ExtractorReader{}, settings, logger)
func NewPlanner(store Store, reader pdffile.Reader, settings Settings, logger *slog.Logger) *Planner {
	return &Planner{store: store, reader: reader, settings: settings, logger: logger, readings: ingest.PDFReadings{}}
}

// Readings are the PDFs planned so far, for the file collector.
func (p *Planner) Readings() ingest.PDFReadings {
	return p.readings
}

// Tally is what the run did so far.
func (p *Planner) Tally() Tally {
	return p.tally
}

// PlanDirectory reads the PDFs under root (files is root opened), seen through
// the same filters as the file collector.
func (p *Planner) PlanDirectory(ctx context.Context, files fs.FS, root string, opts filesource.Options) error {
	return filesource.Walk(ctx, files, root, opts, func(path string, entry fs.DirEntry) error {
		if !pdffile.IsPDF(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", event.FilePathUnder(root, path), err)
		}
		return p.planFile(ctx, files, event.FilePathUnder(root, path), path, info)
	})
}

func (p *Planner) planFile(ctx context.Context, files fs.FS, absolutePath, path string, info fs.FileInfo) error {
	settled, err := p.alreadySettled(ctx, absolutePath, info)
	if err != nil || settled {
		return err
	}
	reading, err := p.readFile(ctx, files, absolutePath, path, info.Size())
	if err != nil {
		return fmt.Errorf("read PDF %q: %w", absolutePath, err)
	}
	p.readings[absolutePath] = reading
	p.tally.add(reading.PDF.Status)
	return nil
}

// alreadySettled skips, without reading the file, a PDF stored at this version
// with a status that a new reading could not improve, and a forgotten one.
func (p *Planner) alreadySettled(ctx context.Context, absolutePath string, info fs.FileInfo) (bool, error) {
	uid := event.StableID(event.SourceFile, absolutePath)
	if forgotten, err := p.store.IsForgotten(ctx, uid); err != nil || forgotten {
		return forgotten, err
	}
	stored, known, err := p.store.StoredEvent(ctx, uid)
	if err != nil || !known {
		return false, err
	}
	revision, _ := stored.Revision()
	return revision == info.ModTime().UnixNano() && stored.PDF().Status != event.PDFNone, nil
}

// readFile returns an error only for a cancelled run: what is wrong with a file
// costs that file.
func (p *Planner) readFile(ctx context.Context, files fs.FS, absolutePath, path string, size int64) (ingest.PDFReading, error) {
	if size > p.settings.MaxBytes {
		return p.unreadable(absolutePath, fmt.Errorf("%d bytes, limit %d", size, p.settings.MaxBytes)), nil
	}
	raw, err := fs.ReadFile(files, path)
	if err != nil {
		return p.unreadable(absolutePath, err), nil
	}
	document, err := p.reader.Open(raw, pdffile.Limits{MaxPages: p.settings.MaxPages})
	if err != nil {
		return p.unreadable(absolutePath, err), nil
	}
	return p.readPages(ctx, document, absolutePath)
}

// unreadable never logs the file's text, only why it could not be read.
func (p *Planner) unreadable(absolutePath string, reason error) ingest.PDFReading {
	p.logger.Debug("PDF not read", "path", absolutePath, "reason", reason.Error())
	return ingest.PDFReading{PDF: event.PDF{Status: event.PDFUnreadable}}
}

func (p *Planner) readPages(ctx context.Context, document pdffile.Document, absolutePath string) (ingest.PDFReading, error) {
	pages := make([]string, document.PageCount())
	pending := 0
	for index := range pages {
		page, err := readPage(ctx, document, index+1)
		if ctx.Err() != nil {
			return ingest.PDFReading{}, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return p.unreadable(absolutePath, fmt.Errorf("page %d took over %v", index+1, pageTimeout)), nil
		}
		if err != nil {
			p.logger.Debug("PDF page not read", "path", absolutePath, "reason", err.Error())
			continue
		}
		pages[index] = page.Text
		if page.Scanned {
			pending++
		}
	}
	return ingest.PDFReading{PDF: statusOf(len(pages), pending), Text: strings.Join(pages, chunking.PageSeparator)}, nil
}

func readPage(ctx context.Context, document pdffile.Document, number int) (pdffile.Page, error) {
	pageCtx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	return document.Page(pageCtx, number)
}

func statusOf(pages, pending int) event.PDF {
	if pending > 0 {
		return event.PDF{Status: event.PDFPending, Pages: pages, PendingPages: pending}
	}
	return event.PDF{Status: event.PDFRead, Pages: pages}
}
