package retrievalsuite

import (
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/llm"
)

// CachingEmbedder remembers vectors by text, on disk, so the scale curve
// (tens of thousands of distractors at ~27 ms each on a GPU) only pays for
// embedding once per model. Key the file by model: vectors of two models
// are not interchangeable.
type CachingEmbedder struct {
	embedder llm.Embedder
	path     string
	vectors  map[[sha256.Size]byte][]float32
	added    int
}

// NewCachingEmbedder loads the cache at path, if any.
//
//	cached, err := retrievalsuite.NewCachingEmbedder(embedder, "~/.cache/cade/eval/nomic.gob")
func NewCachingEmbedder(embedder llm.Embedder, path string) (*CachingEmbedder, error) {
	cache := &CachingEmbedder{embedder: embedder, path: path, vectors: map[[sha256.Size]byte][]float32{}}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cache, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open embedding cache %q: %w", path, err)
	}
	defer file.Close()
	if err := gob.NewDecoder(file).Decode(&cache.vectors); err != nil {
		return nil, fmt.Errorf("decode embedding cache %q, expected a gob map of vectors (delete it to rebuild): %w", path, err)
	}
	return cache, nil
}

// Embed returns the cached vector or computes and remembers it.
func (c *CachingEmbedder) Embed(text string) ([]float32, error) {
	key := sha256.Sum256([]byte(text))
	if vector, found := c.vectors[key]; found {
		return vector, nil
	}
	vector, err := c.embedder.Embed(text)
	if err == nil {
		c.vectors[key], c.added = vector, c.added+1
	}
	return vector, err
}

// Save writes the cache if anything was added.
func (c *CachingEmbedder) Save() error {
	if c.added == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("create embedding cache directory for %q: %w", c.path, err)
	}
	file, err := os.OpenFile(c.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("write embedding cache %q: %w", c.path, err)
	}
	defer file.Close()
	return gob.NewEncoder(file).Encode(c.vectors)
}
