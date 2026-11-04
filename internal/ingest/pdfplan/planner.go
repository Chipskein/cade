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

// pageKeySeparator makes a page's address: no file collector looks one up, so it
// cannot be mistaken for a file.
const pageKeySeparator = "#p="

// Store is what planning reads from the database.
type Store interface {
	IsForgotten(ctx context.Context, uid string) (bool, error)
	StoredEvent(ctx context.Context, uid string) (event.Event, bool, error)
}

// PageDescriber describes the image of a scanned page, under key, its address.
// The status says what became of it: described, waiting for the per-run limit,
// or unreadable. An error is a model failure, and stops the run.
type PageDescriber func(ctx context.Context, key string, raw []byte) (event.Image, error)

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
	describe PageDescriber
	readings ingest.PDFReadings
	tally    Tally
}

// NewPlanner plans against store. Without a describer, as with images off,
// scanned pages wait, and nothing loads the vision model.
//
//	planner := pdfplan.NewPlanner(store, pdffile.ExtractorReader{}, settings, nil, logger)
func NewPlanner(store Store, reader pdffile.Reader, settings Settings, describe PageDescriber, logger *slog.Logger) *Planner {
	return &Planner{store: store, reader: reader, settings: settings, describe: describe, logger: logger, readings: ingest.PDFReadings{}}
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
	if revision != info.ModTime().UnixNano() {
		return false, nil
	}
	return settledStatus(stored.PDF().Status, p.describe != nil), nil
}

// settledStatus says a new reading could not improve this status: a PDF waiting
// on a describer is worth reading again only if there is one now.
func settledStatus(status event.PDFStatus, canDescribe bool) bool {
	switch status {
	case event.PDFRead, event.PDFUnreadable:
		return true
	case event.PDFPending:
		return !canDescribe
	}
	return false
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
	reading := pageReading{planner: p, document: document, path: absolutePath, pages: make([]string, document.PageCount())}
	for index := range reading.pages {
		abandoned, err := reading.readPage(ctx, index+1)
		if err != nil {
			return ingest.PDFReading{}, err
		}
		if abandoned {
			return p.unreadable(absolutePath, fmt.Errorf("page %d took over %v", index+1, pageTimeout)), nil
		}
	}
	return reading.result(), nil
}

// pageReading is one PDF being read, page by page.
type pageReading struct {
	planner  *Planner
	document pdffile.Document
	path     string
	pages    []string
	images   []event.PDFPageImage
	pending  int
}

func (r *pageReading) result() ingest.PDFReading {
	return ingest.PDFReading{PDF: statusOf(len(r.pages), r.pending), Text: strings.Join(r.pages, chunking.PageSeparator), PageImages: r.images}
}

// readPage reports abandoned for a page over the timeout, and an error only for
// a cancelled run or a failed model; a bad page costs that page.
func (r *pageReading) readPage(ctx context.Context, number int) (abandoned bool, err error) {
	page, err := readPage(ctx, r.document, number)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true, nil
	}
	if err != nil {
		r.planner.logger.Debug("PDF page not read", "path", r.path, "page", number, "reason", err.Error())
		return false, nil
	}
	r.pages[number-1] = page.Text
	if !page.Scanned {
		return false, nil
	}
	return r.readScan(ctx, number)
}

func (r *pageReading) readScan(ctx context.Context, number int) (abandoned bool, err error) {
	if r.planner.describe == nil {
		r.pending++
		return false, nil
	}
	raw, err := readScan(ctx, r.document, number)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true, nil
	}
	if err != nil {
		r.planner.logger.Debug("PDF scan not read", "path", r.path, "page", number, "reason", err.Error())
		return false, nil
	}
	return false, r.describeScan(ctx, number, raw)
}

func (r *pageReading) describeScan(ctx context.Context, number int, raw []byte) error {
	image, err := r.planner.describe(ctx, fmt.Sprintf("%s%s%d", r.path, pageKeySeparator, number), raw)
	if err != nil {
		return fmt.Errorf("describe page %d: %w", number, err)
	}
	switch image.Status {
	case event.CaptionPending:
		r.pending++
	case event.CaptionDescribed:
		r.pages[number-1] = strings.TrimSpace(r.pages[number-1] + "\n" + image.SearchableText())
		r.images = append(r.images, event.PDFPageImage{Number: number, Image: image})
	}
	return nil
}

func readScan(ctx context.Context, document pdffile.Document, number int) ([]byte, error) {
	scanCtx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	return document.Scan(scanCtx, number)
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
