package entity

import (
	"maps"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func pdfWithPages(pages ...event.PDFPageImage) event.Event {
	metadata := event.File{Path: "/docs/contrato.pdf"}.Metadata()
	maps.Copy(metadata, event.PDFPageImagesMetadata(pages))
	return event.Event{Source: event.SourceFile, Metadata: metadata}
}

func page(number int, hash string) event.PDFPageImage {
	return event.PDFPageImage{Number: number, Image: event.Image{SHA256: hash}}
}

func TestScannedPagesAreImagesOfTheirOwnRole(t *testing.T) {
	refs := pageImageRefs(pdfWithPages(page(2, "aaa"), page(12, "bbb")))
	want := []Ref{
		{Type: TypeImage, Key: "aaa", Name: "contrato.pdf, p. 2", Role: RolePageImage},
		{Type: TypeImage, Key: "bbb", Name: "contrato.pdf, p. 12", Role: RolePageImage},
	}
	if len(refs) != 2 || refs[0] != want[0] || refs[1] != want[1] {
		t.Errorf("got %+v, want %+v", refs, want)
	}
}

func TestARepeatedPageIsOneRef(t *testing.T) {
	if refs := pageImageRefs(pdfWithPages(page(1, "aaa"), page(2, "aaa"))); len(refs) != 1 {
		t.Errorf("got %+v, want the first page only", refs)
	}
}

func TestRefsOfIncludesThePages(t *testing.T) {
	var images int
	for _, ref := range RefsOf(pdfWithPages(page(1, "aaa"))) {
		if ref.Type == TypeImage {
			images++
			if ref.Role == RoleShows {
				t.Error("a page must not count as an image the file shows")
			}
		}
	}
	if images != 1 {
		t.Errorf("got %d image refs, want 1", images)
	}
}
