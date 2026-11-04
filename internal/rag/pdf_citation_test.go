package rag

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

const threePages = "relatorio.pdf\nmigração do banco\fsegunda página\fconclusão"

func pdfHit(needle string) storage.ScoredEvent {
	metadata := event.File{Path: "/docs/relatorio.pdf"}.Metadata()
	maps.Copy(metadata, event.PDF{Status: event.PDFRead, Pages: 3}.Metadata())
	start := strings.Index(threePages, needle)
	return storage.ScoredEvent{Event: event.Event{Source: event.SourceFile, Content: threePages, Metadata: metadata},
		Chunk: storage.Chunk{Start: start, End: start + len(needle)}, ChunkCount: 3}
}

func TestCitedPageOfAMatchedChunk(t *testing.T) {
	for needle, want := range map[string]int{"migração": 1, "segunda": 2, "conclusão": 3} {
		if page, known := CitedPage(pdfHit(needle)); !known || page != want {
			t.Errorf("%q: page %d, %v; want %d", needle, page, known, want)
		}
	}
}

func TestCitedPageNeedsAPDFAndAChunk(t *testing.T) {
	neighbor := pdfHit("segunda")
	neighbor.Chunk, neighbor.ChunkCount = storage.Chunk{}, 0
	note := pdfHit("segunda")
	note.Event.Metadata = event.File{Path: "/notas/a.md"}.Metadata()
	unreadable := pdfHit("segunda")
	unreadable.Event.Metadata = event.PDF{Status: event.PDFUnreadable}.Metadata()
	for name, hit := range map[string]storage.ScoredEvent{"neighbor": neighbor, "note": note, "unreadable": unreadable} {
		if page, known := CitedPage(hit); known {
			t.Errorf("%s: page %d, want none", name, page)
		}
	}
}

func TestEvidenceNoteNamesTheFileAndThePage(t *testing.T) {
	for wording, want := range map[*NoteWording]string{&PromptWording: "relatorio.pdf, p. 2", &EnglishWording: "relatorio.pdf, p. 2"} {
		if got := EvidenceNote(pdfHit("segunda"), time.UTC, *wording); got != want {
			t.Errorf("note = %q, want %q", got, want)
		}
	}
}

func TestPageNoteAppliesToASingleChunk(t *testing.T) {
	hit := pdfHit("migração")
	hit.ChunkCount = 1
	if got := EvidenceNote(hit, time.UTC, PromptWording); got != "relatorio.pdf, p. 1" {
		t.Errorf("note = %q", got)
	}
}

func TestPromptNeverShowsAPageSeparator(t *testing.T) {
	neighbor := pdfHit("segunda")
	neighbor.Chunk, neighbor.ChunkCount = storage.Chunk{}, 0
	if text := PromptEvidenceText(neighbor); strings.Contains(text, "\f") || !strings.Contains(text, "segunda página") {
		t.Errorf("text = %q", text)
	}
}
