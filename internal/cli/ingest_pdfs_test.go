package cli

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rootfs"
	"github.com/chipskein/cade/internal/testfakes"

	"github.com/chipskein/spfdtextextractor/pdftest"
)

func pdfsWorld(pdfsOn bool) *fakeWorld {
	world := newFakeWorld()
	world.realFiles = true
	world.cfg.Sources.PDFs = pdfsOn
	world.cfg.Sources.Directories = []string{"/home/ana/docs"}
	addFile(world, "/home/ana/docs/relatorio.pdf", pdftest.TextDocument("Introdução", "A migração do banco terminou"))
	addFile(world, "/home/ana/docs/quebrado.pdf", []byte("isto não é um PDF"))
	return world
}

func addFile(world *fakeWorld, path string, data []byte) {
	world.files.MapFS[rootfs.Name(path)] = &fstest.MapFile{Data: data, ModTime: cliNow}
}

func TestIngestReadsPDFsWhenOn(t *testing.T) {
	world := pdfsWorld(true)
	code, stdout, stderr := world.run("ingest", "file")
	if code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	read := storedFile(t, world, "/home/ana/docs/relatorio.pdf")
	if read.Content != "relatorio.pdf\nIntrodução\fA migração do banco terminou" {
		t.Errorf("content = %q", read.Content)
	}
	if read.PDF() != (event.PDF{Status: event.PDFRead, Pages: 2}) {
		t.Errorf("metadata = %+v", read.PDF())
	}
	if bad := storedFile(t, world, "/home/ana/docs/quebrado.pdf"); bad.PDF().Status != event.PDFUnreadable || bad.Content != "quebrado.pdf" {
		t.Errorf("corrupt file = %q, %+v", bad.Content, bad.PDF())
	}
	if !strings.Contains(stdout, "1 lido, 0 com páginas à espera, 1 ilegível") {
		t.Errorf("report = %q", stdout)
	}
}

func TestIngestLeavesPDFsByNameWhenOff(t *testing.T) {
	world := pdfsWorld(false)
	if code, _, stderr := world.run("ingest", "file"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	if off := storedFile(t, world, "/home/ana/docs/relatorio.pdf"); off.Content != "relatorio.pdf" || off.PDF().Status != event.PDFNone {
		t.Errorf("content = %q, %+v", off.Content, off.PDF())
	}
}

func TestTurningPDFsOnReadsWhatWasStoredByName(t *testing.T) {
	world := pdfsWorld(false)
	world.run("ingest", "file")
	world.cfg.Sources.PDFs = true
	if code, _, stderr := world.run("ingest", "file"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	if read := storedFile(t, world, "/home/ana/docs/relatorio.pdf"); !strings.Contains(read.Content, "migração") {
		t.Errorf("content = %q", read.Content)
	}
}

func TestSecondRunDoesNotReadAPDFAgain(t *testing.T) {
	world := pdfsWorld(true)
	world.run("ingest", "file")
	_, stdout, _ := world.run("ingest", "file")
	if strings.Contains(stdout, "PDFs ") {
		t.Errorf("second run read again: %q", stdout)
	}
}

func TestPDFReportInEnglish(t *testing.T) {
	world := pdfsWorld(true)
	world.cfg.UI.Language = "en"
	_, stdout, _ := world.run("ingest", "file")
	if !strings.Contains(stdout, "1 read, 0 with pages waiting, 1 unreadable") {
		t.Errorf("report = %q", stdout)
	}
}

// scannedPDF is a page with no text, only a full-page JPEG.
func scannedPDF(t *testing.T) []byte {
	t.Helper()
	var file bytes.Buffer
	if err := jpeg.Encode(&file, image.NewGray(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	builder := &pdftest.Builder{}
	picture := builder.AddStream("/Subtype/Image/Width 16/Height 16/Filter/DCTDecode", file.Bytes())
	content := builder.AddStream("", []byte("q 612 0 0 792 0 0 cm /Im Do Q"))
	return builder.Bytes(builder.AddPages([]string{fmt.Sprintf("/Contents %d 0 R/Resources<</XObject<</Im %d 0 R>>>>", content, picture)}))
}

func scanWorld(t *testing.T, imagesOn bool) *fakeWorld {
	t.Helper()
	world := newFakeWorld()
	world.realFiles = true
	world.cfg.Sources.PDFs, world.cfg.Sources.Images = true, imagesOn
	world.cfg.Sources.Directories = []string{"/home/ana/docs"}
	world.describer = &testfakes.FakeImageDescriber{Reply: "Description: um contrato assinado\nVisible text: CLÁUSULA 1"}
	addFile(world, "/home/ana/docs/scan.pdf", scannedPDF(t))
	return world
}

func TestAScannedPageIsFoundByWhatItShows(t *testing.T) {
	world := scanWorld(t, true)
	if code, _, stderr := world.run("ingest", "file"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	read := storedFile(t, world, "/home/ana/docs/scan.pdf")
	if !strings.Contains(read.Content, "um contrato assinado") || !strings.Contains(read.Content, "CLÁUSULA 1") {
		t.Errorf("content = %q", read.Content)
	}
	if read.PDF().Status != event.PDFRead || len(read.PDFPageImages()) != 1 {
		t.Errorf("pdf = %+v, pages = %+v", read.PDF(), read.PDFPageImages())
	}
}

func TestWithImagesOffTheVisionModelIsNeverLoaded(t *testing.T) {
	world := scanWorld(t, false)
	if code, stdout, stderr := world.run("ingest", "file"); code != 0 || !strings.Contains(stdout, "1 com páginas à espera") {
		t.Fatalf("code %d: %s %s", code, stdout, stderr)
	}
	if len(world.describer.Images) != 0 {
		t.Errorf("described %d images with images off", len(world.describer.Images))
	}
	if read := storedFile(t, world, "/home/ana/docs/scan.pdf"); read.PDF().Status != event.PDFPending {
		t.Errorf("pdf = %+v", read.PDF())
	}
}

func TestTurningImagesOnDescribesTheWaitingPages(t *testing.T) {
	world := scanWorld(t, false)
	world.run("ingest", "file")
	world.cfg.Sources.Images = true
	if code, _, stderr := world.run("ingest", "file"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	read := storedFile(t, world, "/home/ana/docs/scan.pdf")
	if read.PDF().Status != event.PDFRead || !strings.Contains(read.Content, "um contrato assinado") {
		t.Errorf("pdf = %+v, content = %q", read.PDF(), read.Content)
	}
}

func TestScannedPagesShareThePerRunLimitWithImages(t *testing.T) {
	world := scanWorld(t, true)
	world.cfg.Ingest.MaxImagesPerRun = 1
	addImage(t, world, "/home/ana/docs/print.png", 7)
	if code, _, stderr := world.run("ingest", "file"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	if len(world.describer.Images) != 1 {
		t.Errorf("described %d, want the limit of 1", len(world.describer.Images))
	}
	if read := storedFile(t, world, "/home/ana/docs/scan.pdf"); read.PDF().Status != event.PDFPending {
		t.Errorf("pdf = %+v, the page must wait for the next run", read.PDF())
	}
}
