package benchmarks

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/pdffile"

	"github.com/chipskein/spfdtextextractor/pdftest"
)

// A dense page of a report: about what a full A4 page of text holds.
const (
	benchPDFLines = 45
	benchPDFPages = 20
	benchPDFLine  = "A migração do banco de pedidos terminou sem perda de dados e com a janela prevista."
	benchPDFScan  = "../../testdata/pdfs/contrato-escaneado.pdf"
	benchPDFMax   = 1 << 10
	scanMaxSide   = 1024
	scanMaxPixels = 64_000_000
)

func openBenchPDF(b *testing.B, raw []byte) pdffile.Document {
	b.Helper()
	document, err := pdffile.ExtractorReader{}.Open(raw, pdffile.Limits{MaxPages: benchPDFMax})
	if err != nil {
		b.Fatal(err)
	}
	return document
}

// BenchmarkReadPDFPage is the cost of reading one page of text, without the
// embedding every chunk of it pays afterwards.
func BenchmarkReadPDFPage(b *testing.B) {
	page := strings.TrimSpace(strings.Repeat(benchPDFLine+"\n", benchPDFLines))
	pages := make([]string, benchPDFPages)
	for index := range pages {
		pages[index] = page
	}
	document := openBenchPDF(b, pdftest.TextDocument(pages...))
	number := 0
	for b.Loop() {
		number = number%benchPDFPages + 1
		if _, err := document.Page(context.Background(), number); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDescribeScannedPage is what one scanned page costs an ingest: reading
// its image and describing it, on the fixture of the retrieval suite.
func BenchmarkDescribeScannedPage(b *testing.B) {
	raw, err := os.ReadFile(benchPDFScan)
	if err != nil {
		b.Fatal(err)
	}
	document := openBenchPDF(b, raw)
	describer := loadImageDescriber(b)
	limits := imagefile.Limits{MaxPixels: scanMaxPixels, MaxSide: scanMaxSide}
	for b.Loop() {
		scan, err := document.Scan(context.Background(), 1)
		if err != nil {
			b.Fatal(err)
		}
		describeOnce(b, describer, scan, limits)
	}
}
