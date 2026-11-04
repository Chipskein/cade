package event

import (
	"reflect"
	"testing"
)

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

func describedPages() []PDFPageImage {
	return []PDFPageImage{
		{Number: 2, Image: Image{SHA256: "aaa", Status: CaptionDescribed, Description: "um contrato", VisibleText: "CLÁUSULA 1", Model: "m", PromptVersion: 2}},
		{Number: 12, Image: Image{SHA256: "bbb", Status: CaptionDescribed, Description: "um gráfico", Model: "m", PromptVersion: 2}},
	}
}

func TestPDFPageImagesRoundTrip(t *testing.T) {
	event := Event{Metadata: PDFPageImagesMetadata(describedPages())}
	if got := event.PDFPageImages(); !reflect.DeepEqual(got, describedPages()) {
		t.Errorf("got %+v", got)
	}
}

func TestPDFPageImagesOfOtherEventsAreNone(t *testing.T) {
	if got := (Event{Metadata: Metadata{}}).PDFPageImages(); got != nil {
		t.Errorf("got %+v", got)
	}
	bad := Event{Metadata: Metadata{keyPDFPageImages: "x=aaa;3=;4=bbb"}}
	if got := bad.PDFPageImages(); len(got) != 1 || got[0].Number != 4 {
		t.Errorf("malformed list: got %+v", got)
	}
}

func TestWithoutImageTextKeepsThePagesAndDropsTheirText(t *testing.T) {
	event := Event{Metadata: PDFPageImagesMetadata(describedPages())}
	stored := event.WithoutImageText()
	pages := stored.PDFPageImages()
	if len(pages) != 2 || pages[0].Number != 2 || pages[0].Image.SHA256 != "aaa" {
		t.Fatalf("pages = %+v", pages)
	}
	if pages[0].Image.Description != "" || pages[0].Image.VisibleText != "" || pages[0].Image.Model != "" {
		t.Errorf("the text stayed in the metadata: %+v", pages[0])
	}
	if event.PDFPageImages()[0].Image.Description == "" {
		t.Error("WithoutImageText changed the original event")
	}
}
