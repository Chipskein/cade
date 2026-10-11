package retrievalsuite

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest/filesource"
)

// FixtureDescriber gives the image metadata of a fixture's bytes
// (imagecaption.FixtureCaptions).
type FixtureDescriber interface {
	Describe(ctx context.Context, name string, raw []byte) (event.Image, error)
}

// CaptionImages fills the text and image metadata of the events naming an
// image fixture in directory, as ingestion stores a described image, so
// the suites search the model's real descriptions (phase 19).
//
//	err := corpus.CaptionImages(ctx, "testdata/images", captions)
func (c *Corpus) CaptionImages(ctx context.Context, directory string, describer FixtureDescriber) error {
	for i := range c.Events {
		if c.Events[i].Image == "" {
			continue
		}
		if err := captionEvent(ctx, &c.Events[i], directory, describer); err != nil {
			return fmt.Errorf("corpus event %q: %w", c.Events[i].ID, err)
		}
	}
	return nil
}

func captionEvent(ctx context.Context, corpusEvent *CorpusEvent, directory string, describer FixtureDescriber) error {
	if _, err := imageText(*corpusEvent, ""); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(directory, corpusEvent.Image))
	if err != nil {
		return fmt.Errorf("read image fixture: %w", err)
	}
	image, err := describer.Describe(ctx, corpusEvent.Image, raw)
	if err != nil {
		return err
	}
	metadata := maps.Clone(corpusEvent.Metadata)
	maps.Copy(metadata, image.Metadata())
	text, _ := imageText(*corpusEvent, image.SearchableText())
	corpusEvent.Content, corpusEvent.Metadata = text, metadata
	return nil
}

// imageText places a description as ingestion does: an image file's text
// is its name and the description; a Teams message showing a picture adds
// it after its own text (#24).
func imageText(corpusEvent CorpusEvent, description string) (string, error) {
	ev := event.Event{Metadata: corpusEvent.Metadata}
	switch {
	case corpusEvent.Source == event.SourceFile && ev.File().Path != "":
		return filesource.EventContent(ev.File().Path, description), nil
	case corpusEvent.Source == event.SourceTeams && len(ev.ImageURLs()) > 0:
		return corpusEvent.Content + "\n" + description, nil
	}
	return "", fmt.Errorf("image %q on a %s event, expected a file event with metadata.path or a Teams message with metadata.%s",
		corpusEvent.Image, corpusEvent.Source, event.ImageURLsKey)
}
