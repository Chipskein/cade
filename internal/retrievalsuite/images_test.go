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

func TestCaptionImagesRejectsAnImageOnAnEventWithoutOne(t *testing.T) {
	for _, corpusEvent := range []CorpusEvent{{ID: "x", Source: event.SourceTeams, Image: "erro.png"}, {ID: "y", Source: event.SourceGit, Image: "erro.png"}} {
		corpus := Corpus{Events: []CorpusEvent{corpusEvent}}
		if err := corpus.CaptionImages(context.Background(), t.TempDir(), &FakeFixtureDescriber{}); err == nil {
			t.Errorf("expected an image on %+v rejected", corpusEvent)
		}
	}
}

// A picture pasted in Teams goes after the message's text (#24).
func TestCaptionImagesAddsAPictureToATeamsMessage(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "erro.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	describer := &FakeFixtureDescriber{Image: event.Image{SHA256: "h", Status: event.CaptionDescribed, Description: "um terminal"}}
	message := CorpusEvent{ID: "t", Source: event.SourceTeams, Image: "erro.png", Content: "João: olha\nConversa: chat João, Eu\nRecebida por você",
		Metadata: event.Metadata{event.ImageURLsKey: "https://us-api.asm.skype.com/v1/objects/0-1/views/imgo"}}
	corpus := Corpus{Events: []CorpusEvent{message}}
	if err := corpus.CaptionImages(context.Background(), directory, describer); err != nil {
		t.Fatal(err)
	}
	if got := corpus.Events[0]; got.Content != message.Content+"\nImagem: um terminal" || (event.Event{Metadata: got.Metadata}).Image().SHA256 != "h" {
		t.Fatalf("expected the description after the message, got %+v", got)
	}
}
