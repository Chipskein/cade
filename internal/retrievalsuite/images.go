package retrievalsuite

import (
	"context"
	"fmt"
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
	path := (event.Event{Metadata: corpusEvent.Metadata}).File().Path
	if corpusEvent.Source != event.SourceFile || path == "" {
		return fmt.Errorf("image %q on a %s event with path %q, expected a file event with metadata.path", corpusEvent.Image, corpusEvent.Source, path)
	}
	raw, err := os.ReadFile(filepath.Join(directory, corpusEvent.Image))
	if err != nil {
		return fmt.Errorf("read image fixture: %w", err)
	}
	image, err := describer.Describe(ctx, corpusEvent.Image, raw)
	if err != nil {
		return err
	}
	metadata := event.Metadata{}
	for key, value := range corpusEvent.Metadata {
		metadata[key] = value
	}
	for key, value := range image.Metadata() {
		metadata[key] = value
	}
	corpusEvent.Content, corpusEvent.Metadata = filesource.EventContent(path, image.SearchableText()), metadata
	return nil
}
