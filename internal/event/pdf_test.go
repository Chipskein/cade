package event

import "testing"

func TestPDFMetadataRoundTrips(t *testing.T) {
	want := PDF{Status: PDFPending, Pages: 12, PendingPages: 3}
	if got := (Event{Metadata: want.Metadata()}).PDF(); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPDFOfAnotherEventIsNone(t *testing.T) {
	if got := (Event{Metadata: Metadata{}}).PDF(); got != (PDF{}) {
		t.Errorf("got %+v, want the zero value", got)
	}
	if got := (Event{}).PDF().Status; got != PDFNone {
		t.Errorf("status = %q, want PDFNone", got)
	}
}
