package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rootfs"
	"github.com/chipskein/cade/internal/testfakes"
)

// recaptionWorld has images on disk stored with a description by an older
// model, one per path.
func recaptionWorld(t *testing.T, paths ...string) *fakeWorld {
	t.Helper()
	world := newFakeWorld()
	world.describer = &testfakes.FakeImageDescriber{Reply: "Description: um terminal novo\nVisible text: none"}
	for i, path := range paths {
		addImage(t, world, path, byte(i+1))
		digest := sha256.Sum256(world.files.MapFS[rootfs.Name(path)].Data)
		old := event.Image{SHA256: hex.EncodeToString(digest[:]), Status: event.CaptionDescribed, Model: "old.gguf", PromptVersion: 1, Description: "antiga"}
		metadata := event.File{Path: path, ModifiedAt: cliNow}.Metadata()
		for key, value := range old.Metadata() {
			metadata[key] = value
		}
		world.store.Events = append(world.store.Events, event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile,
			Timestamp: cliNow, Content: "x.png\n" + old.SearchableText(), Metadata: metadata})
	}
	return world
}

func TestReindexCaptionsDescribesOutdatedImagesAgain(t *testing.T) {
	world := recaptionWorld(t, "/home/ana/prints/erro.png")
	code, stdout, stderr := world.run("reindex", "--captions")
	if code != 0 || !strings.Contains(stdout, "1 imagem descrita de novo com Qwen3.5-2B-Q4_K_M.gguf.") {
		t.Fatalf("expected one image described again, got %d %q %q", code, stdout, stderr)
	}
	stored := world.store.Events[0]
	if stored.Image().Model != "Qwen3.5-2B-Q4_K_M.gguf" || !strings.Contains(stored.Content, "um terminal novo") || strings.Contains(stored.Content, "antiga") {
		t.Fatalf("expected the new description stored, got %+v", stored)
	}
	if !world.describer.Closed || world.describerOpenAtEmbedderLoad || len(world.embedder.Inputs) == 0 {
		t.Fatalf("expected the vision model freed before embedding the new text, got open=%v embeds=%d", world.describerOpenAtEmbedderLoad, len(world.embedder.Inputs))
	}
}

func TestReindexCaptionsWorksInBatchesOfThePerRunLimit(t *testing.T) {
	world := recaptionWorld(t, "/home/ana/prints/a.png", "/home/ana/prints/b.png", "/home/ana/prints/c.png")
	world.cfg.Ingest.MaxImagesPerRun = 2
	if code, stdout, _ := world.run("reindex", "--captions"); code != 0 || !strings.Contains(stdout, "3 imagens descritas de novo") {
		t.Fatalf("expected all three described, got %d %q", code, stdout)
	}
	if world.describerLoads != 2 || world.embedderLoads != 2 {
		t.Fatalf("expected two batches, each loading each model once, got %d vision and %d embedder loads", world.describerLoads, world.embedderLoads)
	}
}

func TestReindexCaptionsSkipsAChangedFile(t *testing.T) {
	world := recaptionWorld(t, "/home/ana/prints/erro.png")
	addImage(t, world, "/home/ana/prints/erro.png", 99)
	_, stdout, _ := world.run("reindex", "--captions")
	if !strings.Contains(stdout, "0 imagens descritas de novo") || !strings.Contains(stdout, "1 pulada: o arquivo sumiu ou mudou") || world.embedderLoads != 0 {
		t.Fatalf("expected the changed file skipped without embedding, got %q (%d embedder loads)", stdout, world.embedderLoads)
	}
}

func TestReindexCaptionsWithNothingOutdatedLoadsNoModel(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit}
	if code, stdout, _ := world.run("reindex", "--captions"); code != 0 || world.describerLoads != 0 || world.embedderLoads != 0 || !strings.Contains(stdout, "0 imagens descritas de novo") {
		t.Fatalf("expected nothing done and no model loaded, got %d %q", code, stdout)
	}
}
