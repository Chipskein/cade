package event

import "strconv"

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
