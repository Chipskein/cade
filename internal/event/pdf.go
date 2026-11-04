package event

import (
	"fmt"
	"strconv"
	"strings"
)

// PDFStatus is where a PDF file's reading stands (#108).
type PDFStatus string

const (
	// PDFNone: not a PDF, or PDFs are off.
	PDFNone PDFStatus = ""
	// PDFRead: the event's text holds every page that has content.
	PDFRead PDFStatus = "read"
	// PDFPending: some scanned pages wait to be described, for a later
	// `ingest` (the per-run limit) or for images to be turned on.
	PDFPending PDFStatus = "pending"
	// PDFUnreadable: encrypted, corrupt or above a limit; tried again only
	// when the file changes.
	PDFUnreadable PDFStatus = "unreadable"
)

// PDF is the PDF part of a file event's metadata.
type PDF struct {
	Status       PDFStatus
	Pages        int
	PendingPages int
}

const (
	keyPDFStatus       = "pdf_status"
	keyPDFPages        = "pdf_pages"
	keyPDFPendingPages = "pdf_pending_pages"
)

// Metadata is the stored form of p, merged into a file's metadata.
func (p PDF) Metadata() Metadata {
	return Metadata{keyPDFStatus: string(p.Status), keyPDFPages: strconv.Itoa(p.Pages), keyPDFPendingPages: strconv.Itoa(p.PendingPages)}
}

// PDF reads e's metadata as a PDF; Status is PDFNone for a file that is not
// one. A chunk's page is known only for such a file.
func (e Event) PDF() PDF {
	pages, _ := strconv.Atoi(e.Metadata[keyPDFPages])
	pending, _ := strconv.Atoi(e.Metadata[keyPDFPendingPages])
	return PDF{Status: PDFStatus(e.Metadata[keyPDFStatus]), Pages: pages, PendingPages: pending}
}

// PDFPageImage is a scanned page, as an image with its description. The store
// keeps its text as representations of the Image, as for an image file; the
// event keeps the page and the hash.
type PDFPageImage struct {
	Number int
	Image  Image
}

// Metadata keys of a PDF's scanned pages. keyPDFPageImages lists "12=hash;13=hash".
// The text of each page has a key of its own, not one encoded value, because the
// masks of privacy.Event rewrite every metadata value as text.
const (
	keyPDFPageImages     = "pdf_page_images"
	pdfPageTextKeyPrefix = "pdf_page_text_"
	pageImagePairs       = ";"
	pageImagePair        = "="
	pageDescription      = "description"
	pageVisibleText      = "visible"
	pageModel            = "model"
	pagePromptVersion    = "prompt"
)

func pdfPageTextKey(number int, field string) string {
	return fmt.Sprintf("%s%d_%s", pdfPageTextKeyPrefix, number, field)
}

// PDFPageImagesMetadata is the stored form of pages, merged into a file's
// metadata.
func PDFPageImagesMetadata(pages []PDFPageImage) Metadata {
	metadata := Metadata{}
	pairs := make([]string, len(pages))
	for index, page := range pages {
		pairs[index] = strconv.Itoa(page.Number) + pageImagePair + page.Image.SHA256
		metadata[pdfPageTextKey(page.Number, pageDescription)] = page.Image.Description
		metadata[pdfPageTextKey(page.Number, pageVisibleText)] = page.Image.VisibleText
		metadata[pdfPageTextKey(page.Number, pageModel)] = page.Image.Model
		metadata[pdfPageTextKey(page.Number, pagePromptVersion)] = strconv.Itoa(page.Image.PromptVersion)
	}
	metadata[keyPDFPageImages] = strings.Join(pairs, pageImagePairs)
	return metadata
}

// PDFPageImages reads e's scanned pages. A stored event has only the page and
// the hash; its text lives in representations (WithoutImageText).
//
//	for _, page := range ev.PDFPageImages() { ... }
func (e Event) PDFPageImages() []PDFPageImage {
	listed := e.Metadata[keyPDFPageImages]
	if listed == "" {
		return nil
	}
	var pages []PDFPageImage
	for pair := range strings.SplitSeq(listed, pageImagePairs) {
		number, hash, _ := strings.Cut(pair, pageImagePair)
		if page, err := strconv.Atoi(number); err == nil && hash != "" {
			pages = append(pages, e.pdfPageImage(page, hash))
		}
	}
	return pages
}

func (e Event) pdfPageImage(number int, hash string) PDFPageImage {
	prompt, _ := strconv.Atoi(e.Metadata[pdfPageTextKey(number, pagePromptVersion)])
	return PDFPageImage{Number: number, Image: Image{SHA256: hash, Status: CaptionDescribed,
		Description: e.Metadata[pdfPageTextKey(number, pageDescription)], VisibleText: e.Metadata[pdfPageTextKey(number, pageVisibleText)],
		Model: e.Metadata[pdfPageTextKey(number, pageModel)], PromptVersion: prompt}}
}
