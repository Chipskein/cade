// Package evalimages opens the descriptions of the image fixtures
// (testdata/images) for the model-backed suites (phase 19): the caption
// suite writes them, and the retrieval and injection suites reuse them.
package evalimages

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/llm/llamacpp"
)

// The variables the mage eval targets set to the model paths.
const (
	GenerationModelEnv = "CADE_TEST_GENERATION_MODEL"
	VisionProjectorEnv = "CADE_TEST_VISION_PROJECTOR"
)

// errNoVisionModel is returned when a fixture is not cached and the model
// paths are not set.
var errNoVisionModel = errors.New("image fixture not in the caption cache and " + GenerationModelEnv + " or " + VisionProjectorEnv +
	" not set; run `go tool mage evalCaptions` first or set both")

// Open returns the fixture descriptions cached for the generation model in
// GenerationModelEnv, loading the model only for a fixture not cached.
//
//	captions, err := evalimages.Open()
//	defer captions.Close()
func Open() (*imagecaption.FixtureCaptions, error) {
	modelPath, projectorPath := os.Getenv(GenerationModelEnv), os.Getenv(VisionProjectorEnv)
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate the cache directory: %w", err)
	}
	model := filepath.Base(modelPath)
	path := imagecaption.FixtureCachePath(filepath.Join(cacheDir, "cade", "eval"), model)
	return imagecaption.OpenFixtureCaptions(path, loader(modelPath, projectorPath), model, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func loader(modelPath, projectorPath string) imagecaption.DescriberLoader {
	return func() (imagecaption.ClosableDescriber, error) {
		if modelPath == "" || projectorPath == "" {
			return nil, errNoVisionModel
		}
		defaults := config.Defaults()
		opts := llamacpp.ModelOptions{Path: modelPath, ContextTokens: defaults.Vision.ContextTokens, GPULayers: defaults.Generation.GPULayers}
		return llamacpp.LoadImageDescriber(opts, projectorPath)
	}
}
