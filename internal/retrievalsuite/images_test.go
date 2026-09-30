package retrievalsuite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

// FakeFixtureDescriber describes every fixture as Image and records names.
type FakeFixtureDescriber struct {
	Image event.Image
	Names []string
}

func (f *FakeFixtureDescriber) Describe(_ context.Context, name string, _ []byte) (event.Image, error) {
	f.Names = append(f.Names, name)
	return f.Image, nil
}

func TestCaptionImagesFillsTheTextOfImageEventsOnly(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "erro.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	describer := &FakeFixtureDescriber{Image: event.Image{Status: event.CaptionDescribed, Description: "um terminal"}}
	corpus := Corpus{Events: []CorpusEvent{
		{ID: "img", Source: event.SourceFile, Image: "erro.png", Metadata: event.Metadata{"path": "/home/eu/prints/erro.png"}},
		{ID: "note", Source: event.SourceFile, Content: "nota", Metadata: event.Metadata{"path": "/home/eu/nota.md"}},
	}}
	if err := corpus.CaptionImages(context.Background(), directory, describer); err != nil {
		t.Fatal(err)
	}
	image := corpus.Events[0]
	if image.Content != "erro.png\nImagem: um terminal" || (event.Event{Metadata: image.Metadata}).Image().Status != event.CaptionDescribed {
		t.Fatalf("expected the description as text and metadata, got %+v", image)
	}
	if corpus.Events[1].Content != "nota" || len(describer.Names) != 1 {
		t.Fatalf("expected other events untouched, got %+v and %v", corpus.Events[1], describer.Names)
	}
}

func TestCaptionImagesRejectsAnImageOnANonFileEvent(t *testing.T) {
	corpus := Corpus{Events: []CorpusEvent{{ID: "x", Source: event.SourceTeams, Image: "erro.png"}}}
	if err := corpus.CaptionImages(context.Background(), t.TempDir(), &FakeFixtureDescriber{}); err == nil {
		t.Fatal("expected an image on a Teams event to be rejected")
	}
}
