package imagecaption

import (
	"context"
	"log/slog"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/llm"
)

// lazyDescriber turns image bytes into a described event.Image, loading the
// model on first use: shared by ingestion and `reindex --captions`.
type lazyDescriber struct {
	load   DescriberLoader
	model  string
	logger *slog.Logger
	loaded ClosableDescriber
}

func newLazyDescriber(load DescriberLoader, model string, logger *slog.Logger) *lazyDescriber {
	return &lazyDescriber{load: load, model: model, logger: logger}
}

// imageFor marks an image that does not decode as unreadable, so one
// damaged file never stops the run; a model failure does stop it.
func (d *lazyDescriber) imageFor(ctx context.Context, path string, raw []byte, hash string) (event.Image, outcome, error) {
	decoded, err := imagefile.Decode(raw, decodeLimits)
	if err != nil {
		d.logger.Debug("image not decoded", "path", path, "error", err.Error())
		return event.Image{SHA256: hash, Status: event.CaptionUnreadable}, outcomeUnreadable, nil
	}
	caption, err := d.describe(ctx, decoded.Image)
	if err != nil {
		return event.Image{}, outcomeUnreadable, err
	}
	return event.Image{SHA256: hash, Width: decoded.Original.Width, Height: decoded.Original.Height, Status: event.CaptionDescribed,
		Model: d.model, PromptVersion: PromptVersion, Description: caption.Description, VisibleText: caption.VisibleText}, outcomeDescribed, nil
}

func (d *lazyDescriber) describe(ctx context.Context, image llm.RGBImage) (Caption, error) {
	if d.loaded == nil {
		describer, err := d.load()
		if err != nil {
			return Caption{}, err
		}
		d.loaded = describer
	}
	reply, err := d.loaded.DescribeImage(ctx, image, Instructions, MaxTokens)
	if err != nil {
		return Caption{}, err
	}
	return ParseCaption(reply), nil
}

// Close frees the model if it was loaded; the next image loads it again.
func (d *lazyDescriber) Close() error {
	if d.loaded == nil {
		return nil
	}
	err := d.loaded.Close()
	d.loaded = nil
	return err
}
