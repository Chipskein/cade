package ingest

import (
	"maps"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func pdfVersion(modified time.Time, pdf event.PDF) event.Event {
	metadata := event.File{Path: "/docs/a.pdf", ModifiedAt: modified}.Metadata()
	if pdf.Status != event.PDFNone {
		maps.Copy(metadata, pdf.Metadata())
	}
	return event.Event{Source: event.SourceFile, Metadata: metadata}
}

func TestAnUnchangedPDFIsReplacedOnlyWhenReadBetter(t *testing.T) {
	none := event.PDF{}
	read := event.PDF{Status: event.PDFRead, Pages: 3}
	unreadable := event.PDF{Status: event.PDFUnreadable}
	waitingTwo := event.PDF{Status: event.PDFPending, Pages: 3, PendingPages: 2}
	waitingOne := event.PDF{Status: event.PDFPending, Pages: 3, PendingPages: 1}
	cases := map[string]struct {
		stored, incoming event.PDF
		want             bool
	}{
		"name only, now read":       {none, read, true},
		"name only, now waiting":    {none, waitingTwo, true},
		"name only, now unreadable": {none, unreadable, true},
		"name only, still":          {none, none, false},
		"fewer pages waiting":       {waitingTwo, waitingOne, true},
		"same pages waiting":        {waitingOne, waitingOne, false},
		"more pages waiting":        {waitingOne, waitingTwo, false},
		"all pages described":       {waitingOne, read, true},
		"waiting, PDFs turned off":  {waitingOne, none, false},
		"read, PDFs turned off":     {read, none, false},
		"read, read again":          {read, read, false},
		"unreadable, read":          {unreadable, read, false},
	}
	modified := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	for name, test := range cases {
		if got := replaces(pdfVersion(modified, test.incoming), pdfVersion(modified, test.stored)); got != test.want {
			t.Errorf("%s: replaces = %v, want %v", name, got, test.want)
		}
	}
}

func TestAChangedPDFIsAlwaysReplaced(t *testing.T) {
	modified := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	stored := pdfVersion(modified, event.PDF{Status: event.PDFRead, Pages: 1})
	if !replaces(pdfVersion(modified.Add(time.Hour), event.PDF{}), stored) {
		t.Error("a newer file replaces what was stored, read or not")
	}
}
