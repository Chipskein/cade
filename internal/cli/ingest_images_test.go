package cli

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rootfs"
	"github.com/chipskein/cade/internal/testfakes"
)

// fakeToken has the shape of a GitHub token; the fixture never holds a
// real one.
const fakeToken = "ghp_0123456789abcdefghijABCDEFGHIJ"

// imagesWorld has ~/prints with one screenshot and one note, and a model
// that reads a token in the screenshot.
func imagesWorld(t *testing.T, imagesOn bool) *fakeWorld {
	t.Helper()
	world := newFakeWorld()
	world.realFiles = true
	world.cfg.Sources.Images = imagesOn
	world.cfg.Sources.Directories = []string{"/home/ana/prints"}
	world.describer = &testfakes.FakeImageDescriber{Reply: "Description: um terminal\nVisible text: export GITHUB_TOKEN=" + fakeToken}
	addImage(t, world, "/home/ana/prints/terminal.png", 1)
	world.files.MapFS[rootfs.Name("/home/ana/prints/notas.md")] = &fstest.MapFile{Data: []byte("revisar o deploy"), ModTime: cliNow}
	return world
}

func addImage(t *testing.T, world *fakeWorld, path string, seed byte) {
	t.Helper()
	picture := image.NewGray(image.Rect(0, 0, 8, 8))
	picture.Pix[0] = seed
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	world.files.MapFS[rootfs.Name(path)] = &fstest.MapFile{Data: encoded.Bytes(), ModTime: cliNow}
}

func storedFile(t *testing.T, world *fakeWorld, path string) event.Event {
	t.Helper()
	for _, ev := range world.store.Events {
		if ev.Source == event.SourceFile && ev.File().Path == path {
			return ev
		}
	}
	t.Fatalf("no file event for %q among %d events", path, len(world.store.Events))
	return event.Event{}
}

func TestIngestDescribesImagesBeforeLoadingTheEmbedder(t *testing.T) {
	world := imagesWorld(t, true)
	code, stdout, stderr := world.run("ingest", "file")
	if code != 0 || world.describerLoads != 1 || !world.describer.Closed || world.describerOpenAtEmbedderLoad {
		t.Fatalf("expected the model loaded once and freed before the embedder, got %d loads, open at embedder load %v (%d %q)",
			world.describerLoads, world.describerOpenAtEmbedderLoad, code, stderr)
	}
	ev := storedFile(t, world, "/home/ana/prints/terminal.png")
	if !strings.Contains(ev.Content, "Imagem: um terminal") || ev.Image().Status != event.CaptionDescribed {
		t.Fatalf("expected the description as the event text, got %+v", ev)
	}
	if !strings.Contains(stdout, "imagens  1 descrita, 0 reaproveitadas, 0 para depois, 0 ilegíveis") {
		t.Fatalf("expected the image report, got %q", stdout)
	}
}

// Acceptance (phase 19): a token read from a screenshot never reaches the
// database, in the text or in the stored transcription.
func TestIngestMasksSecretsReadFromImages(t *testing.T) {
	world := imagesWorld(t, true)
	world.run("ingest", "file")
	ev := storedFile(t, world, "/home/ana/prints/terminal.png")
	if strings.Contains(ev.Content, fakeToken) || strings.Contains(ev.Image().VisibleText, fakeToken) || !strings.Contains(ev.Content, "[redacted:github-token]") {
		t.Fatalf("expected the token masked everywhere, got content %q and visible text %q", ev.Content, ev.Image().VisibleText)
	}
}

func TestIngestWithImagesOffKeepsImagesAsNames(t *testing.T) {
	world := imagesWorld(t, false)
	world.run("ingest", "file")
	if ev := storedFile(t, world, "/home/ana/prints/terminal.png"); world.describerLoads != 0 || ev.Content != "terminal.png" {
		t.Fatalf("expected no model and the name only, got %d loads and %q", world.describerLoads, ev.Content)
	}
}

func TestIngestAgainDescribesNothingAndPrintsNoImageReport(t *testing.T) {
	world := imagesWorld(t, true)
	world.run("ingest", "file")
	world.describer.Closed = false
	_, stdout, _ := world.run("ingest", "file")
	if world.describerLoads != 1 || strings.Contains(stdout, "imagens") {
		t.Fatalf("expected no second load and no image report, got %d loads, %q", world.describerLoads, stdout)
	}
}

// Files are walked in lexical order, so ultimo.png is the one left over.
func TestIngestSaysHowToContinueAfterThePerRunLimit(t *testing.T) {
	world := imagesWorld(t, true)
	world.cfg.Ingest.MaxImagesPerRun = 1
	addImage(t, world, "/home/ana/prints/ultimo.png", 2)
	_, stdout, _ := world.run("ingest", "file")
	if !strings.Contains(stdout, "1 para depois") || !strings.Contains(stdout, "próximas execuções de `cade ingest file`") {
		t.Fatalf("expected one image left for later and how to continue, got %q", stdout)
	}
	if storedFile(t, world, "/home/ana/prints/ultimo.png").Image().Status != event.CaptionPending {
		t.Fatal("expected the second image stored as pending")
	}
}

func TestIngestOfOtherSourcesNeverLoadsTheVisionModel(t *testing.T) {
	world := imagesWorld(t, true)
	if code, _, stderr := world.run("ingest", "git"); code != 0 || world.describerLoads != 0 {
		t.Fatalf("expected git ingestion without the vision model, got %d loads (%d %q)", world.describerLoads, code, stderr)
	}
}
