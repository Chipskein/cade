package imagecaption

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

// recaptionWorld has /prints/erro.png on disk, stored with an old
// description by another model.
type recaptionWorld struct {
	files       fstest.MapFS
	describer   *testfakes.FakeImageDescriber
	stored      event.Event
	recaptioner *Recaptioner
}

func newRecaptionWorld(t *testing.T) *recaptionWorld {
	t.Helper()
	file := pngFile(t, 30, 10, 1)
	digest := sha256.Sum256(file.Data)
	old := event.Image{SHA256: hex.EncodeToString(digest[:]), Status: event.CaptionDescribed, Model: "old.gguf", Description: "descrição antiga"}
	stored := storedImage("/prints/erro.png", old)
	stored.Content = "erro.png\n" + old.SearchableText()
	stored.Metadata["custom"] = "kept"
	world := &recaptionWorld{files: fstest.MapFS{"prints/erro.png": file}, describer: &testfakes.FakeImageDescriber{Reply: fakeReply}, stored: stored}
	load := func() (ClosableDescriber, error) { return world.describer, nil }
	world.recaptioner = NewRecaptioner(world.files, load, "Qwen3.5-2B-Q4_K_M.gguf", slog.New(slog.NewJSONHandler(io.Discard, nil)))
	return world
}

func TestRecaptionReplacesTheDescriptionAndKeepsTheRest(t *testing.T) {
	world := newRecaptionWorld(t)
	updated, recaptioned, err := world.recaptioner.Recaption(context.Background(), world.stored)
	if err != nil || !recaptioned {
		t.Fatalf("expected a new description, got %v (err %v)", recaptioned, err)
	}
	image := updated.Image()
	if image.Model != "Qwen3.5-2B-Q4_K_M.gguf" || image.PromptVersion != PromptVersion || image.Width != 30 || image.Description != "um terminal" {
		t.Fatalf("expected the current model and prompt, got %+v", image)
	}
	if updated.UID != world.stored.UID || updated.Metadata["custom"] != "kept" || strings.Contains(updated.Content, "antiga") ||
		!strings.HasPrefix(updated.Content, "erro.png\nImagem: um terminal") {
		t.Fatalf("expected the same event with the new text, got %+v", updated)
	}
	if world.stored.Metadata[event.CaptionModelKey] != "old.gguf" {
		t.Fatal("expected the stored event left unchanged")
	}
}

func TestRecaptionSkipsAChangedOrMissingFile(t *testing.T) {
	world := newRecaptionWorld(t)
	world.files["prints/erro.png"] = pngFile(t, 30, 10, 2)
	if _, recaptioned, err := world.recaptioner.Recaption(context.Background(), world.stored); err != nil || recaptioned {
		t.Fatalf("expected a changed file skipped, got %v (err %v)", recaptioned, err)
	}
	delete(world.files, "prints/erro.png")
	if _, recaptioned, err := world.recaptioner.Recaption(context.Background(), world.stored); err != nil || recaptioned || len(world.describer.Images) != 0 {
		t.Fatalf("expected a missing file skipped without the model, got %v (err %v)", recaptioned, err)
	}
}
