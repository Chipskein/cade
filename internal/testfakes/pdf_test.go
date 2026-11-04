package testfakes

import (
	"context"
	"errors"
	"testing"

	"github.com/chipskein/cade/internal/pdffile"
)

func TestFakePDFReaderOpensOnlyWhatItKnows(t *testing.T) {
	known := &FakePDFDocument{Pages: []pdffile.Page{{Number: 1, Text: "um"}}}
	reader := &FakePDFReader{Documents: map[string]*FakePDFDocument{"raw": known}}
	document, err := reader.Open([]byte("raw"), pdffile.Limits{MaxPages: 3})
	if err != nil || document.PageCount() != 1 {
		t.Fatalf("Open = %v, %v", document, err)
	}
	if _, err := reader.Open([]byte("other"), pdffile.Limits{}); !errors.Is(err, pdffile.ErrUnreadable) {
		t.Errorf("error = %v, want ErrUnreadable", err)
	}
	if len(reader.Opened) != 2 || reader.Opened[0].MaxPages != 3 {
		t.Errorf("Opened = %+v", reader.Opened)
	}
}

func TestFakePDFDocumentServesAndCountsReads(t *testing.T) {
	document := &FakePDFDocument{
		Pages:           []pdffile.Page{{Number: 1, Text: "um"}, {Number: 2, Scanned: true}},
		Scans:           map[int][]byte{2: []byte("scan")},
		UnreadablePages: map[int]bool{3: true},
	}
	ctx := context.Background()
	if page, err := document.Page(ctx, 1); err != nil || page.Text != "um" {
		t.Errorf("Page(1) = %+v, %v", page, err)
	}
	if scan, err := document.Scan(ctx, 2); err != nil || string(scan) != "scan" {
		t.Errorf("Scan(2) = %q, %v", scan, err)
	}
	if _, err := document.Scan(ctx, 1); !errors.Is(err, pdffile.ErrUnreadable) {
		t.Errorf("Scan(1): error = %v", err)
	}
	if _, err := document.Page(ctx, 3); !errors.Is(err, pdffile.ErrUnreadable) {
		t.Errorf("Page(3): error = %v", err)
	}
	if document.PageReads != 2 || document.ScanReads != 2 {
		t.Errorf("reads = %d pages, %d scans", document.PageReads, document.ScanReads)
	}
}

func TestFakePDFDocumentHonorsTheContext(t *testing.T) {
	document := &FakePDFDocument{Pages: []pdffile.Page{{Number: 1}}, Scans: map[int][]byte{1: {}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := document.Page(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Page: error = %v", err)
	}
	if _, err := document.Scan(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Scan: error = %v", err)
	}
}
