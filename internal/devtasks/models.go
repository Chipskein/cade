package devtasks

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Model is a GGUF file the model-backed tests and evaluations load.
type Model int

const (
	EmbeddingModel Model = iota
	GenerationModel
	// VisionProjector is the mmproj of the generation model (phase 19).
	VisionProjector
	// RerankerModel is only for `go tool mage evalRerank` (phase 17
	// experiment); no command uses it.
	RerankerModel
)

const (
	embeddingModelURL = "https://huggingface.co/nomic-ai/nomic-embed-text-v2-moe-GGUF/resolve/main/nomic-embed-text-v2-moe.Q4_K_M.gguf"
	// Qwen publishes no GGUF of Qwen3.5; unsloth's conversion is pinned to a
	// commit because the repository is re-uploaded when templates are fixed.
	qwen35URL          = "https://huggingface.co/unsloth/Qwen3.5-2B-GGUF/resolve/f6d5376be1edb4d416d56da11e5397a961aca8ae"
	generationModelURL = qwen35URL + "/Qwen3.5-2B-Q4_K_M.gguf"
	visionProjectorURL = qwen35URL + "/mmproj-F16.gguf"
	rerankerModelURL   = "https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/main/bge-reranker-v2-m3-Q4_K_M.gguf"
	partialSuffix      = ".part"
)

type modelSource struct {
	url string
	// fileName differs from the URL's for the projector: every size's file is
	// named mmproj-F16.gguf, so the local copy carries the model's name.
	fileName string
	// pathVar overrides the local path, e.g. to calibrate another embedder.
	pathVar EnvVar
}

var modelSources = map[Model]modelSource{
	EmbeddingModel:  {embeddingModelURL, "nomic-embed-text-v2-moe.Q4_K_M.gguf", EnvEmbeddingModel},
	GenerationModel: {generationModelURL, "Qwen3.5-2B-Q4_K_M.gguf", EnvGenerationModel},
	VisionProjector: {visionProjectorURL, "mmproj-Qwen3.5-2B-F16.gguf", EnvVisionProjector},
	RerankerModel:   {rerankerModelURL, "bge-reranker-v2-m3-Q4_K_M.gguf", EnvRerankerModel},
}

// runtimeModels are what `cade` itself uses, downloaded by `go tool mage models`.
var runtimeModels = []Model{EmbeddingModel, GenerationModel, VisionProjector}

func modelPaths(env Environment, modelsDir string) map[Model]string {
	paths := make(map[Model]string, len(modelSources))
	for model, source := range modelSources {
		paths[model] = settingOr(env, source.pathVar, filepath.Join(modelsDir, source.fileName))
	}
	return paths
}

// ModelFetcher downloads a URL; HTTPFetcher is the real one.
type ModelFetcher interface {
	Fetch(url string, destination io.Writer) error
}

// HTTPFetcher downloads over HTTP(S), following redirects.
type HTTPFetcher struct {
	Client *http.Client
}

// Fetch copies the body of url into destination.
func (f HTTPFetcher) Fetch(url string, destination io.Writer) error {
	response, err := f.Client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: got HTTP %s, want 200 OK", url, response.Status)
	}
	_, err = io.Copy(destination, response.Body)
	return err
}

// Models downloads the models cade uses that are not there yet.
func (t *Tasks) Models() error {
	return t.EnsureModels(runtimeModels...)
}

// EnsureModels downloads each missing model.
func (t *Tasks) EnsureModels(models ...Model) error {
	for _, model := range models {
		if fileExists(t.settings.ModelPath(model)) {
			continue
		}
		if err := t.download(modelSources[model].url, t.settings.ModelPath(model)); err != nil {
			return err
		}
	}
	return nil
}

// download writes to a .part file first, so an interrupted download is not
// taken for a model on the next run.
func (t *Tasks) download(url, destination string) error {
	fmt.Fprintf(t.progress, "downloading %s\n", url)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	partial, err := os.Create(destination + partialSuffix)
	if err != nil {
		return err
	}
	fetchErr := t.fetcher.Fetch(url, partial)
	if closeErr := partial.Close(); fetchErr == nil {
		fetchErr = closeErr
	}
	if fetchErr != nil {
		return fmt.Errorf("download %s to %s: %w", url, destination, fetchErr)
	}
	return os.Rename(destination+partialSuffix, destination)
}
