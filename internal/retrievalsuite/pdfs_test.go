package retrievalsuite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/chunking"
	"github.com/chipskein/cade/internal/event"

	"github.com/chipskein/spfdtextextractor/pdftest"
)

const pdfFixtures = "../../testdata/pdfs"

func describedContract() *FakeFixtureDescriber {
	return &FakeFixtureDescriber{Image: event.Image{SHA256: "scan", Status: event.CaptionDescribed, Description: "um contrato escaneado"}}
}

func pdfEventOf(id, fixture string) CorpusEvent {
	return CorpusEvent{ID: id, Source: event.SourceFile, PDF: fixture, Metadata: event.File{Path: "/home/eu/docs/" + fixture}.Metadata()}
}

func TestShippedReportCitesThePageOfAPhrase(t *testing.T) {
	corpus := Corpus{Events: []CorpusEvent{pdfEventOf("r", "relatorio-migracao.pdf")}}
	if err := corpus.ReadPDFs(context.Background(), pdfFixtures, describedContract()); err != nil {
		t.Fatal(err)
	}
	report := corpus.Events[0]
	if !strings.HasPrefix(report.Content, "relatorio-migracao.pdf\n") {
		t.Errorf("content = %q", report.Content)
	}
	if got := chunking.PageOf(report.Content, strings.Index(report.Content, "40 minutos")); got != 2 {
		t.Errorf("the phrase is on page %d, want 2", got)
	}
	if pdf := (event.Event{Metadata: report.Metadata}).PDF(); pdf != (event.PDF{Status: event.PDFRead, Pages: 3}) {
		t.Errorf("pdf = %+v", pdf)
	}
}

func TestShippedScanIsReadOnlyThroughItsDescription(t *testing.T) {
	corpus := Corpus{Events: []CorpusEvent{pdfEventOf("c", "contrato-escaneado.pdf")}}
	if err := corpus.ReadPDFs(context.Background(), pdfFixtures, describedContract()); err != nil {
		t.Fatal(err)
	}
	contract := corpus.Events[0]
	if contract.Content != "contrato-escaneado.pdf\nImagem: um contrato escaneado" {
		t.Errorf("content = %q", contract.Content)
	}
	if pages := (event.Event{Metadata: contract.Metadata}).PDFPageImages(); len(pages) != 1 || pages[0].Number != 1 {
		t.Errorf("pages = %+v", pages)
	}
}

func TestReadPDFsRejectsWhatItCannotPlace(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "bom.pdf"), pdftest.TextDocument("texto"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ruim.pdf"), []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]CorpusEvent{
		"missing fixture":  pdfEventOf("a", "nada.pdf"),
		"unreadable":       pdfEventOf("b", "ruim.pdf"),
		"not a file event": {ID: "c", Source: event.SourceTeams, PDF: "bom.pdf", Metadata: event.Metadata{}},
		"no path":          {ID: "d", Source: event.SourceFile, PDF: "bom.pdf", Metadata: event.Metadata{}},
	}
	for name, corpusEvent := range cases {
		corpus := Corpus{Events: []CorpusEvent{corpusEvent}}
		if err := corpus.ReadPDFs(context.Background(), directory, describedContract()); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestReadPDFsLeavesOtherEventsAlone(t *testing.T) {
	other := CorpusEvent{ID: "n", Source: event.SourceFile, Content: "nota", Metadata: event.File{Path: "/n.md"}.Metadata()}
	corpus := Corpus{Events: []CorpusEvent{other}}
	if err := corpus.ReadPDFs(context.Background(), pdfFixtures, describedContract()); err != nil {
		t.Fatal(err)
	}
	if corpus.Events[0].Content != "nota" {
		t.Errorf("content = %q", corpus.Events[0].Content)
	}
}

func TestStoppedContextStopsReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Corpus{}).ReadPDFs(ctx, pdfFixtures, describedContract()); err == nil {
		t.Error("expected the cancellation")
	}
}
