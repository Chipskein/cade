// Package pdffile reads PDF pages for the file source (#108). It wraps
// spfdtextextractor so no other package imports it.
package pdffile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"

	"github.com/chipskein/spfdtextextractor"
)

// A page counts as scanned when it shows almost no text and one image covers
// most of it, so a slide with a big photo and a caption keeps its caption as
// text and a page with only a logo is not sent to the vision model.
const (
	scannedMaxTextChars = 16
	scannedMinCover     = 0.5
)

// The rotations a page can declare, clockwise.
const (
	rotationQuarter = 90
	rotationHalf    = 180
	rotationThree   = 270
)

// ErrUnreadable is a PDF or page cade cannot read: encrypted, corrupt, above a
// limit, or a scan in a format without a decoder. It never stops an ingest.
var ErrUnreadable = errors.New("unreadable PDF")

// Limits bound what one file may cost.
type Limits struct {
	MaxPages int
}

// Page is one page's reading. Scanned says its content is an image, to be
// fetched with Document.Scan.
type Page struct {
	Number  int
	Text    string
	Scanned bool
}

// Document is an opened PDF; only one call at a time may use it.
type Document interface {
	PageCount() int
	Page(ctx context.Context, number int) (Page, error)
	// Scan is the scanned image of a page as a jpeg or png file.
	Scan(ctx context.Context, number int) ([]byte, error)
}

// Reader opens PDFs.
type Reader interface {
	Open(raw []byte, limits Limits) (Document, error)
}

// ExtractorReader is the Reader over spfdtextextractor.
type ExtractorReader struct{}

// Open reads the structure of raw, which must stay unchanged while in use.
//
//	document, err := pdffile.ExtractorReader{}.Open(raw, pdffile.Limits{MaxPages: 500})
func (ExtractorReader) Open(raw []byte, limits Limits) (Document, error) {
	opened, err := spfdtextextractor.Open(raw, spfdtextextractor.Limits{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if opened.PageCount() > limits.MaxPages {
		return nil, fmt.Errorf("%w: %d pages, limit %d", ErrUnreadable, opened.PageCount(), limits.MaxPages)
	}
	return extractorDocument{opened: opened}, nil
}

type extractorDocument struct {
	opened *spfdtextextractor.Document
}

func (d extractorDocument) PageCount() int {
	return d.opened.PageCount()
}

func (d extractorDocument) Page(ctx context.Context, number int) (Page, error) {
	page, err := d.read(ctx, number)
	if err != nil {
		return Page{}, err
	}
	_, scanned := scanOf(page)
	return Page{Number: number, Text: page.Text, Scanned: scanned}, nil
}

func (d extractorDocument) Scan(ctx context.Context, number int) ([]byte, error) {
	page, err := d.read(ctx, number)
	if err != nil {
		return nil, err
	}
	scan, scanned := scanOf(page)
	if !scanned {
		return nil, fmt.Errorf("%w: page %d has no scanned image", ErrUnreadable, number)
	}
	raw, err := encodedScan(scan, page.Rotate)
	if err != nil {
		return nil, fmt.Errorf("%w: page %d: %w", ErrUnreadable, number, err)
	}
	return raw, nil
}

// read keeps a cancelled run apart from a bad page: the first stops the
// ingest, the second costs that page.
func (d extractorDocument) read(ctx context.Context, number int) (spfdtextextractor.Page, error) {
	page, err := d.opened.Page(ctx, number)
	if err == nil {
		return page, nil
	}
	if ctx.Err() != nil {
		return spfdtextextractor.Page{}, ctx.Err()
	}
	return spfdtextextractor.Page{}, fmt.Errorf("%w: page %d: %w", ErrUnreadable, number, err)
}

func scanOf(page spfdtextextractor.Page) (spfdtextextractor.Image, bool) {
	if len([]rune(strings.TrimSpace(page.Text))) >= scannedMaxTextChars {
		return spfdtextextractor.Image{}, false
	}
	var largest spfdtextextractor.Image
	for _, candidate := range page.Images {
		if candidate.PageCover > largest.PageCover {
			largest = candidate
		}
	}
	return largest, largest.PageCover >= scannedMinCover
}

// encodedScan keeps an upright JPEG as its file: the same page in another PDF
// then has the same bytes, and its description is reused by hash.
func encodedScan(scan spfdtextextractor.Image, rotate int) ([]byte, error) {
	if scan.Format == spfdtextextractor.FormatJPEG && rotate == 0 {
		return scan.Data()
	}
	picture, err := scan.Decode()
	if err != nil {
		return nil, err
	}
	var file bytes.Buffer
	if err := png.Encode(&file, rotated(picture, rotate)); err != nil {
		return nil, fmt.Errorf("encode a %v image as png: %w", picture.Bounds().Size(), err)
	}
	return file.Bytes(), nil
}

// rotated turns picture clockwise; the vision model reads a sideways scan badly.
func rotated(picture image.Image, degrees int) image.Image {
	if degrees != rotationQuarter && degrees != rotationHalf && degrees != rotationThree {
		return picture
	}
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	turned := image.NewNRGBA(rotatedBounds(width, height, degrees))
	for y := range height {
		for x := range width {
			column, row := rotatedPoint(x, y, width, height, degrees)
			turned.Set(column, row, picture.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return turned
}

func rotatedBounds(width, height, degrees int) image.Rectangle {
	if degrees == rotationHalf {
		return image.Rect(0, 0, width, height)
	}
	return image.Rect(0, 0, height, width)
}

func rotatedPoint(x, y, width, height, degrees int) (int, int) {
	switch degrees {
	case rotationQuarter:
		return height - 1 - y, x
	case rotationHalf:
		return width - 1 - x, height - 1 - y
	}
	return y, width - 1 - x
}
