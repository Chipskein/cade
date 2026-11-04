package pdffile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/chipskein/spfdtextextractor/pdftest"
)

const fullPage = "q 612 0 0 792 0 0 cm /Im Do Q"

func openPDF(t *testing.T, raw []byte) Document {
	t.Helper()
	document, err := ExtractorReader{}.Open(raw, Limits{MaxPages: 10})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func jpegFile(t *testing.T) []byte {
	t.Helper()
	var file bytes.Buffer
	if err := jpeg.Encode(&file, image.NewGray(image.Rect(0, 0, 4, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return file.Bytes()
}

// imagePage builds a one-page PDF drawing an image, with text before it.
func imagePage(imageEntries string, data []byte, drawing, pageEntries string) []byte {
	builder := &pdftest.Builder{}
	font := builder.Add("<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>")
	picture := builder.AddStream("/Subtype/Image"+imageEntries, data)
	content := builder.AddStream("", []byte(drawing))
	return builder.Bytes(builder.AddPages([]string{fmt.Sprintf("%s/Contents %d 0 R/Resources<</Font<</F1 %d 0 R>>/XObject<</Im %d 0 R>>>>", pageEntries, content, font, picture)}))
}

func TestPageReadsText(t *testing.T) {
	document := openPDF(t, pdftest.TextDocument("Relatório da migração do banco", "Página dois"))
	if document.PageCount() != 2 {
		t.Fatalf("PageCount = %d, want 2", document.PageCount())
	}
	page, err := document.Page(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if page != (Page{Number: 2, Text: "Página dois"}) {
		t.Errorf("page = %+v", page)
	}
}

func TestScannedPageDetection(t *testing.T) {
	longText := "BT /F1 12 Tf (uma legenda longa o bastante para ser texto) Tj ET "
	cases := map[string]struct {
		drawing string
		want    bool
	}{
		"image covering the page": {fullPage, true},
		"a short scanner stamp":   {"BT /F1 12 Tf (p. 1) Tj ET " + fullPage, true},
		"text and a big image":    {longText + fullPage, false},
		"a small logo":            {"q 50 0 0 50 0 0 cm /Im Do Q", false},
		"no image at all":         {"BT /F1 12 Tf (x) Tj ET", false},
	}
	entries := "/Width 4/Height 2/Filter/DCTDecode"
	for name, test := range cases {
		document := openPDF(t, imagePage(entries, jpegFile(t), test.drawing, ""))
		page, err := document.Page(context.Background(), 1)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if page.Scanned != test.want {
			t.Errorf("%s: Scanned = %v, want %v", name, page.Scanned, test.want)
		}
	}
}

func TestScanKeepsAnUprightJPEGAsItsFile(t *testing.T) {
	file := jpegFile(t)
	document := openPDF(t, imagePage("/Width 4/Height 2/Filter/DCTDecode", file, fullPage, ""))
	got, err := document.Scan(context.Background(), 1)
	if err != nil || !bytes.Equal(got, file) {
		t.Errorf("Scan changed the file: %d bytes, %v", len(got), err)
	}
}

func TestScanEncodesOtherFormatsAsPNG(t *testing.T) {
	entries := "/Width 2/Height 1/BitsPerComponent 8/ColorSpace/DeviceGray"
	document := openPDF(t, imagePage(entries, []byte{10, 200}, fullPage, ""))
	got, err := document.Scan(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Size() != image.Pt(2, 1) || picture.At(1, 0) != (color.Gray{Y: 200}) {
		t.Errorf("picture = %v, pixel %v", picture.Bounds(), picture.At(1, 0))
	}
}

func TestScanTurnsARotatedPage(t *testing.T) {
	entries := "/Width 2/Height 1/BitsPerComponent 8/ColorSpace/DeviceGray"
	document := openPDF(t, imagePage(entries, []byte{10, 200}, fullPage, "/Rotate 90"))
	got, err := document.Scan(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Size() != image.Pt(1, 2) {
		t.Errorf("size = %v, want a turned 1x2", picture.Bounds().Size())
	}
}

func TestRotatedPutsEachCornerInPlace(t *testing.T) {
	source := image.NewGray(image.Rect(0, 0, 2, 1))
	source.SetGray(0, 0, color.Gray{Y: 200})
	cases := map[int]image.Point{
		0:   image.Pt(0, 0),
		45:  image.Pt(0, 0),
		90:  image.Pt(0, 0),
		180: image.Pt(1, 0),
		270: image.Pt(0, 1),
	}
	for degrees, bright := range cases {
		red, _, _, _ := rotated(source, degrees).At(bright.X, bright.Y).RGBA()
		if red>>8 != 200 {
			t.Errorf("%d degrees: pixel %v = %d, want the bright corner", degrees, bright, red>>8)
		}
	}
}

func TestUnreadableFilesAndPages(t *testing.T) {
	encrypted := bytes.Replace(pdftest.TextDocument("x"), []byte("/Size"), []byte("/Encrypt 9 0 R/Size"), 1)
	for name, raw := range map[string][]byte{"corrupt": []byte("not a pdf"), "encrypted": encrypted} {
		if _, err := (ExtractorReader{}).Open(raw, Limits{MaxPages: 10}); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: error = %v, want ErrUnreadable", name, err)
		}
	}
	_, err := ExtractorReader{}.Open(pdftest.TextDocument("a", "b"), Limits{MaxPages: 1})
	if !errors.Is(err, ErrUnreadable) || !strings.Contains(err.Error(), "2 pages, limit 1") {
		t.Errorf("too many pages: error = %v", err)
	}
}

func TestScanOfUnsupportedFormatIsUnreadable(t *testing.T) {
	document := openPDF(t, imagePage("/Width 4/Height 2/Filter/JBIG2Decode", []byte("x"), fullPage, ""))
	page, err := document.Page(context.Background(), 1)
	if err != nil || !page.Scanned {
		t.Fatalf("page = %+v, %v; a page shown as an image is scanned even without a decoder", page, err)
	}
	if _, err := document.Scan(context.Background(), 1); !errors.Is(err, ErrUnreadable) {
		t.Errorf("error = %v, want ErrUnreadable", err)
	}
}

func TestScanOfATextPageIsUnreadable(t *testing.T) {
	document := openPDF(t, pdftest.TextDocument("texto suficiente para não ser um escaneado"))
	if _, err := document.Scan(context.Background(), 1); !errors.Is(err, ErrUnreadable) {
		t.Errorf("error = %v, want ErrUnreadable", err)
	}
}

func TestPageOutOfRangeIsUnreadable(t *testing.T) {
	document := openPDF(t, pdftest.TextDocument("x"))
	if _, err := document.Page(context.Background(), 9); !errors.Is(err, ErrUnreadable) {
		t.Errorf("Page: error = %v, want ErrUnreadable", err)
	}
	if _, err := document.Scan(context.Background(), 9); !errors.Is(err, ErrUnreadable) {
		t.Errorf("Scan: error = %v, want ErrUnreadable", err)
	}
}

// A cancelled run must stop the ingest, not mark the file unreadable forever.
func TestCancelledContextIsNotUnreadable(t *testing.T) {
	builder := &pdftest.Builder{}
	content := builder.AddStream("", []byte(strings.Repeat("q Q ", 8192)))
	document := openPDF(t, builder.Bytes(builder.AddPages([]string{fmt.Sprintf("/Contents %d 0 R", content)})))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := document.Page(ctx, 1)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnreadable) {
		t.Errorf("error = %v, want only context.Canceled", err)
	}
}
