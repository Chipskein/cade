package imagecaption

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/rootfs"
)

// Recaptioner describes stored images again with the current model and
// prompt: `cade reindex --captions`, after either changed.
type Recaptioner struct {
	files     fs.FS
	describer *lazyDescriber
}

// NewRecaptioner reads the images from files, the filesystem at "/" (see
// rootfs), and describes them as model.
//
//	recaptioner := imagecaption.NewRecaptioner(os.DirFS("/"), loadDescriber, "Qwen3.5-2B-Q4_K_M.gguf", logger)
func NewRecaptioner(files fs.FS, load DescriberLoader, model string, logger *slog.Logger) *Recaptioner {
	return &Recaptioner{files: files, describer: newLazyDescriber(load, model, logger)}
}

// WithModelLoading reports when the vision model starts loading, once, for
// the first image of the run that needs describing.
func (r *Recaptioner) WithModelLoading(onLoad func()) *Recaptioner {
	r.describer.onLoad = onLoad
	return r
}

// Recaption returns stored with a new description, or false when its file
// is gone or its bytes changed: the next `ingest` describes that version.
func (r *Recaptioner) Recaption(ctx context.Context, stored event.Event) (event.Event, bool, error) {
	path := stored.File().Path
	raw, err := fs.ReadFile(r.files, rootfs.Name(path))
	if errors.Is(err, fs.ErrNotExist) {
		return event.Event{}, false, nil
	}
	if err != nil {
		return event.Event{}, false, fmt.Errorf("read image %q: %w", path, err)
	}
	hash := sha256Hex(raw)
	if hash != stored.Image().SHA256 {
		return event.Event{}, false, nil
	}
	image, _, err := r.describer.imageFor(ctx, path, raw, hash)
	if err != nil {
		return event.Event{}, false, fmt.Errorf("describe image %q: %w", path, err)
	}
	return withImage(stored, image), true, nil
}

// withImage replaces the image metadata and the text of a stored event.
func withImage(stored event.Event, image event.Image) event.Event {
	metadata := make(event.Metadata, len(stored.Metadata))
	for key, value := range stored.Metadata {
		metadata[key] = value
	}
	for key, value := range image.Metadata() {
		metadata[key] = value
	}
	stored.Metadata = metadata
	stored.Content = filesource.EventContent(stored.File().Path, image.SearchableText())
	return stored
}

// Close frees the model, if it was loaded.
func (r *Recaptioner) Close() error {
	return r.describer.Close()
}
