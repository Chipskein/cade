package cli

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rootfs"

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
