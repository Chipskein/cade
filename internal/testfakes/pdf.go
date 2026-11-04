package testfakes

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/pdffile"
)

// FakePDFDocument is a PDF's pages as a test wants them read; a page number in
// Scans holds that page's image, and one in UnreadablePages fails.
type FakePDFDocument struct {
	Pages           []pdffile.Page
	Scans           map[int][]byte
	UnreadablePages map[int]bool
	PageReads       int
	ScanReads       int
}

// PageCount is the number of Pages.
func (f *FakePDFDocument) PageCount() int {
	return len(f.Pages)
}

// Page returns Pages[number-1] and counts the read.
func (f *FakePDFDocument) Page(ctx context.Context, number int) (pdffile.Page, error) {
	f.PageReads++
	if err := ctx.Err(); err != nil {
		return pdffile.Page{}, err
	}
	if f.UnreadablePages[number] || number < 1 || number > len(f.Pages) {
		return pdffile.Page{}, fmt.Errorf("%w: fake page %d", pdffile.ErrUnreadable, number)
	}
	return f.Pages[number-1], nil
}

// Scan returns Scans[number] and counts the read.
func (f *FakePDFDocument) Scan(ctx context.Context, number int) ([]byte, error) {
	f.ScanReads++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scan, known := f.Scans[number]
	if !known {
		return nil, fmt.Errorf("%w: fake page %d has no scan", pdffile.ErrUnreadable, number)
	}
	return scan, nil
}

// FakePDFReader opens Documents by the file's bytes; other bytes are unreadable.
type FakePDFReader struct {
	Documents map[string]*FakePDFDocument
	Opened    []pdffile.Limits
}

// Open returns the document registered under raw.
func (f *FakePDFReader) Open(raw []byte, limits pdffile.Limits) (pdffile.Document, error) {
	f.Opened = append(f.Opened, limits)
	document, known := f.Documents[string(raw)]
	if !known {
		return nil, fmt.Errorf("%w: fake file of %d bytes", pdffile.ErrUnreadable, len(raw))
	}
	return document, nil
}
