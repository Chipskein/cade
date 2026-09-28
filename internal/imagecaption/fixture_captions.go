package imagecaption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/event"
)

// FixtureCaptions describes the evaluation fixtures, remembering each
// description on disk by image hash, so the retrieval and injection suites
// reuse what `evalCaptions` wrote and load the vision model only for an
// image it has not described. Key the file by model and prompt version
// (FixtureCachePath): descriptions of two models are not interchangeable.
type FixtureCaptions struct {
	path      string
	images    map[string]event.Image
	added     int
	describer *lazyDescriber
	// Refresh describes every image again, ignoring the cache
	// (evalCaptions measures the model, not the cache).
	Refresh bool
}

// FixtureCachePath is the cache file for model and the current prompt.
//
//	path := imagecaption.FixtureCachePath("~/.cache/cade/eval", "Qwen3.5-2B-Q4_K_M.gguf")
func FixtureCachePath(directory, model string) string {
	return filepath.Join(directory, fmt.Sprintf("captions-%s-prompt%d.json", model, PromptVersion))
}

// OpenFixtureCaptions loads the cache at path, if any.
func OpenFixtureCaptions(path string, load DescriberLoader, model string, logger *slog.Logger) (*FixtureCaptions, error) {
	cache := &FixtureCaptions{path: path, images: map[string]event.Image{}, describer: newLazyDescriber(load, model, logger)}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read caption cache %q: %w", path, err)
	}
	if err := json.Unmarshal(raw, &cache.images); err != nil {
		return nil, fmt.Errorf("decode caption cache %q, expected a JSON object of images by hash (delete it to rebuild): %w", path, err)
	}
	return cache, nil
}

// Describe returns the image metadata for the file's bytes, from the cache
// or the model.
func (f *FixtureCaptions) Describe(ctx context.Context, name string, raw []byte) (event.Image, error) {
	hash := sha256Hex(raw)
	if cached, found := f.images[hash]; found && !f.Refresh {
		return cached, nil
	}
	image, _, err := f.describer.imageFor(ctx, name, raw, hash)
	if err != nil {
		return event.Image{}, fmt.Errorf("describe fixture %q: %w", name, err)
	}
	f.images[hash], f.added = image, f.added+1
	return image, nil
}

// Save writes the cache if anything was described.
func (f *FixtureCaptions) Save() error {
	if f.added == 0 {
		return nil
	}
	encoded, err := json.MarshalIndent(f.images, "", "  ")
	if err != nil {
		return fmt.Errorf("encode caption cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("create caption cache directory for %q: %w", f.path, err)
	}
	return os.WriteFile(f.path, encoded, 0o600)
}

// Close frees the model, if it was loaded.
func (f *FixtureCaptions) Close() error {
	return f.describer.Close()
}
