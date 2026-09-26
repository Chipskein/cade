// Package config loads the user's settings: which sources to ingest, where
// the models and database live (RNF6).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is the full on-disk configuration.
type Config struct {
	DatabasePath string          `json:"database_path"`
	Embedding    EmbeddingConfig `json:"embedding"`
	Generation   ModelConfig     `json:"generation"`
	Retrieval    RetrievalConfig `json:"retrieval"`
	Sources      SourcesConfig   `json:"sources"`
}

// ModelConfig locates a GGUF model and sets how it runs.
type ModelConfig struct {
	ModelPath     string `json:"model_path"`
	ContextTokens int    `json:"context_tokens"`
	Threads       int    `json:"threads"`
	// GPULayers is how many layers to offload to the GPU in CUDA builds;
	// negative means all. CPU builds ignore it.
	GPULayers int `json:"gpu_layers"`
}

// EmbeddingConfig adds the task prefixes some embedding models are trained
// with (nomic-embed-text expects "search_query: " / "search_document: ").
type EmbeddingConfig struct {
	ModelConfig
	QueryPrefix    string `json:"query_prefix"`
	DocumentPrefix string `json:"document_prefix"`
}

// RetrievalConfig tunes semantic search and answer generation.
type RetrievalConfig struct {
	TopK int `json:"top_k"`
	// MaxDistance drops hits whose cosine distance exceeds it, so unrelated
	// questions yield "not found" instead of a forced answer (CA9).
	MaxDistance     float64 `json:"max_distance"`
	MaxAnswerTokens int     `json:"max_answer_tokens"`
}

// SourcesConfig lists the default ingestion targets per source.
type SourcesConfig struct {
	GitRepositories  []string `json:"git_repositories"`
	GitAuthors       []string `json:"git_authors"`
	BrowserHistories []string `json:"browser_histories"`
	// TeamsIndexedDBDirs are the *.indexeddb.leveldb directories of the
	// Teams web client inside a Chromium profile.
	TeamsIndexedDBDirs []string `json:"teams_indexeddb_dirs"`
	Directories        []string `json:"directories"`
	IgnoredDirNames    []string `json:"ignored_dir_names"`
	MaxFileBytes       int64    `json:"max_file_bytes"`
}

// DefaultPath is $XDG_CONFIG_HOME/cade/config.json (or ~/.config/...).
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "cade", "config.json"), nil
}

// Load reads the file at path over the defaults. A missing file is not an
// error: the defaults are returned so `cade` works before `cade init`.
//
//	cfg, err := config.Load("/home/me/.config/cade/config.json")
func Load(path string) (Config, error) {
	cfg := Defaults()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg.expandPaths()
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q, expected a JSON object like `cade init` writes: %w", path, err)
	}
	return cfg.expandPaths()
}

// WriteDefault creates a config file with the defaults, refusing to
// overwrite an existing one.
func WriteDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config %q already exists; edit it instead", path)
	}
	encoded, err := json.MarshalIndent(Defaults(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory for %q: %w", path, err)
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}

func (c Config) expandPaths() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("locate home directory: %w", err)
	}
	c.DatabasePath = expandHome(c.DatabasePath, home)
	c.Embedding.ModelPath = expandHome(c.Embedding.ModelPath, home)
	c.Generation.ModelPath = expandHome(c.Generation.ModelPath, home)
	c.Sources.GitRepositories = expandHomeAll(c.Sources.GitRepositories, home)
	c.Sources.BrowserHistories = expandHomeAll(c.Sources.BrowserHistories, home)
	c.Sources.Directories = expandHomeAll(c.Sources.Directories, home)
	c.Sources.TeamsIndexedDBDirs = expandHomeAll(c.Sources.TeamsIndexedDBDirs, home)
	return c, nil
}

// ExpandHome replaces a leading "~" with the user's home directory; used for
// paths typed on the command line as well as config values.
func ExpandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return expandHome(path, home)
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func expandHomeAll(paths []string, home string) []string {
	expanded := make([]string, len(paths))
	for i, path := range paths {
		expanded[i] = expandHome(path, home)
	}
	return expanded
}
