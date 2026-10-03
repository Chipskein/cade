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
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/sitestorage"
)

// Config is the full on-disk configuration.
type Config struct {
	DatabasePath string          `json:"database_path"`
	Embedding    EmbeddingConfig `json:"embedding"`
	Generation   ModelConfig     `json:"generation"`
	Retrieval    RetrievalConfig `json:"retrieval"`
	Sources      SourcesConfig   `json:"sources"`
	Tasks        TasksConfig     `json:"tasks"`
	UI           UIConfig        `json:"ui"`
	Ingest       IngestConfig    `json:"ingest"`
	Vision       VisionConfig    `json:"vision"`
}

type IngestConfig struct {
	Redact    bool            `json:"redact"`
	Retention RetentionConfig `json:"retention"`
	// MaxImageBytes skips larger images: they are kept, like other
	// binaries, with their name only.
	MaxImageBytes int64 `json:"max_image_bytes"`
	// MaxImagesPerRun bounds how many new images one `ingest` describes;
	// the rest wait for the next runs, so a folder of thousands of photos
	// does not hold up the first ingestion for hours on a CPU.
	MaxImagesPerRun int              `json:"max_images_per_run"`
	Background      BackgroundConfig `json:"background"`
}

// VisionConfig locates the image encoder of the generation model, which
// `ingest` loads to describe images (phase 19); `ask` never does.
type VisionConfig struct {
	// ProjectorPath is the mmproj GGUF released with the generation model.
	ProjectorPath string `json:"projector_path"`
	// ContextTokens is the generator's context while describing: an image
	// scaled to 1024 px takes at most ~1000 tokens and the reply 384, so it
	// can be smaller than generation.context_tokens, and uses less memory.
	ContextTokens int `json:"context_tokens"`
}

type RetentionConfig struct {
	MaxAgeDays map[string]int `json:"max_age_days"`
}

// UIConfig sets the language of the CLI's labels and help, and how
// numeric dates in questions are read.
type UIConfig struct {
	// Language is "auto" (follow the locale), "pt" or "en". Answers to
	// `ask` follow the question's language regardless.
	Language string `json:"language"`
	// DateOrder is "auto" (month first for en_US, day first otherwise),
	// "dmy" or "mdy": whether "12/08" in a question is 12 August or
	// December 8. Output dates are ISO either way.
	DateOrder string `json:"date_order"`
}

// UILanguages are the accepted ui.language values.
var UILanguages = []string{"auto", "pt", "en"}

// UIDateOrders are the accepted ui.date_order values.
var UIDateOrders = []string{"auto", "dmy", "mdy"}

// TasksConfig tells `cade tasks` how to recognize task links.
type TasksConfig struct {
	// TaskURLPatterns are regexes over URLs and message text; the capture
	// groups identify a task, the last one being the identifier written in
	// branch names and PR titles ("162", "PROJ-123").
	TaskURLPatterns []string `json:"task_url_patterns"`
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

// ModelName identifies a model by its file name, which carries the
// quantization (Q4 and Q8 vectors differ too); the database records the
// embedding model's to refuse mixing vectors of two models, and each image
// description the generation model's.
func (m ModelConfig) ModelName() string {
	return filepath.Base(m.ModelPath)
}

// RetrievalConfig tunes semantic search and answer generation.
type RetrievalConfig struct {
	TopK int `json:"top_k"`
	// MaxDistance drops hits whose cosine distance exceeds it, so unrelated
	// questions yield "not found" instead of a forced answer (CA9).
	MaxDistance float64 `json:"max_distance"`
	// MaxBestDistance rejects a whole unfiltered question when even its
	// closest event is farther: nothing in the history answers it.
	MaxBestDistance float64 `json:"max_best_distance"`
	MaxAnswerTokens int     `json:"max_answer_tokens"`
	// Mode is "hybrid" (vector and keyword search fused), "vector" or
	// "lexical".
	Mode string `json:"mode"`
	// MaxFilteredEvents is how many events a person or direction filter
	// may match and still be ranked one by one; 0 means no limit.
	MaxFilteredEvents int `json:"max_filtered_events"`
}

// SourcesConfig lists the default ingestion targets per source.
type SourcesConfig struct {
	GitRepositories []string `json:"git_repositories"`
	GitAuthors      []string `json:"git_authors"`
	// GitIdentities are the user's commit emails or names; "auto" reads
	// git config user.email and user.name of each repository. Commits by
	// anyone else are kept but marked as someone else's.
	GitIdentities    []string `json:"git_identities"`
	BrowserHistories []string `json:"browser_histories"`
	// TeamsIndexedDBDirs are the *.indexeddb.leveldb directories of the
	// Teams web client inside a Chromium profile.
	TeamsIndexedDBDirs []string `json:"teams_indexeddb_dirs"`
	// IndexedDBSchemaDir holds the IndexedDB schemas, one <name>.json each,
	// generated by `cade schema-discover` and editable by hand.
	IndexedDBSchemaDir string `json:"indexeddb_schema_dir"`
	// IndexedDBDirs are, per schema name, the IndexedDB directories it
	// reads (Chromium *.indexeddb.leveldb or Firefox idb).
	IndexedDBDirs map[string][]string `json:"indexeddb_dirs"`
	// RequestCacheURLs are the only URLs read from the browsers' request
	// caches (HTTP cache, Cache API), as named patterns: a request cache
	// holds the responses of every site, and the name is what a schema's
	// records.container selects.
	RequestCacheURLs map[string][]string `json:"request_cache_urls"`
	// StorageOrigins are the only origins read from localStorage and the
	// Origin Private File System: a profile's localStorage holds every
	// site, session tokens among them.
	StorageOrigins   []string `json:"storage_origins"`
	Directories      []string `json:"directories"`
	IgnoredDirNames  []string `json:"ignored_dir_names"`
	IgnoredFileGlobs []string `json:"ignored_file_globs"`
	MaxFileBytes     int64    `json:"max_file_bytes"`
	// Images turns on describing png, jpeg and webp files of Directories
	// with the local vision model, so they are found by what they show.
	Images bool `json:"images"`
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
	cfg.migratePreviousGenerationDefault()
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	return cfg.expandPaths()
}

// migratePreviousGenerationDefault moves configs written by cade init before
// v0.0.0 to the current generation model. Other model paths remain user-set.
func (c *Config) migratePreviousGenerationDefault() {
	if c.Generation.ModelPath == previousGenerationModelPath {
		c.Generation.ModelPath = defaultGenerationModelPath
	}
}

func (c Config) validate() error {
	if err := c.UI.validate(); err != nil {
		return err
	}
	if err := c.Ingest.Background.validate(c.Sources.Images); err != nil {
		return err
	}
	if _, err := requestcache.NewScope(c.Sources.RequestCacheURLs); err != nil {
		return fmt.Errorf("sources.request_cache_urls: %w", err)
	}
	if _, err := sitestorage.NewScope(c.Sources.StorageOrigins); err != nil {
		return fmt.Errorf("sources.storage_origins: %w", err)
	}
	if !c.Sources.Images {
		return nil
	}
	return c.Ingest.validateImageLimits()
}

// validateImageLimits rejects limits that would describe nothing, a
// mistake that would otherwise only show as images never being found.
func (ingest IngestConfig) validateImageLimits() error {
	if ingest.MaxImageBytes <= 0 {
		return fmt.Errorf("ingest.max_image_bytes is %d, expected a positive byte count with sources.images on", ingest.MaxImageBytes)
	}
	if ingest.MaxImagesPerRun <= 0 {
		return fmt.Errorf("ingest.max_images_per_run is %d, expected a positive count with sources.images on", ingest.MaxImagesPerRun)
	}
	return nil
}

func (ui UIConfig) validate() error {
	if !slices.Contains(UILanguages, ui.Language) {
		return fmt.Errorf("ui.language is %q, expected one of %v", ui.Language, UILanguages)
	}
	if !slices.Contains(UIDateOrders, ui.DateOrder) {
		return fmt.Errorf("ui.date_order is %q, expected one of %v", ui.DateOrder, UIDateOrders)
	}
	return nil
}

// Write creates the config file at path with cfg, owner-only since it
// names the user's repositories and profiles. It refuses to overwrite an
// existing file (O_EXCL, so a file created meanwhile is not clobbered).
//
//	err := config.Write("/home/me/.config/cade/config.json", config.Defaults())
func Write(path string, cfg Config) error {
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory for %q: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("config %q already exists; edit it instead", path)
	}
	if err != nil {
		return fmt.Errorf("create config %q: %w", path, err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write config %q: %w", path, err)
	}
	return file.Close()
}

func (c Config) expandPaths() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("locate home directory: %w", err)
	}
	c.DatabasePath = ExpandHomeIn(c.DatabasePath, home)
	c.Embedding.ModelPath = ExpandHomeIn(c.Embedding.ModelPath, home)
	c.Generation.ModelPath = ExpandHomeIn(c.Generation.ModelPath, home)
	c.Vision.ProjectorPath = ExpandHomeIn(c.Vision.ProjectorPath, home)
	c.Sources.GitRepositories = expandHomeAll(c.Sources.GitRepositories, home)
	c.Sources.BrowserHistories = expandHomeAll(c.Sources.BrowserHistories, home)
	c.Sources.Directories = expandHomeAll(c.Sources.Directories, home)
	c.Sources.TeamsIndexedDBDirs = expandHomeAll(c.Sources.TeamsIndexedDBDirs, home)
	c.Sources.IndexedDBSchemaDir = ExpandHomeIn(c.Sources.IndexedDBSchemaDir, home)
	c.Sources.IndexedDBDirs = expandHomeInMap(c.Sources.IndexedDBDirs, home)
	return c, nil
}

// ExpandHome replaces a leading "~" with the user's home directory; used for
// paths typed on the command line as well as config values.
func ExpandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return ExpandHomeIn(path, home)
}

// ContractHome writes a path under home as "~/...", like the defaults,
// so the file stays valid if the home directory moves.
//
//	config.ContractHome("/home/me/src/app", "/home/me") // "~/src/app"
func ContractHome(path, home string) string {
	if path == home {
		return "~"
	}
	if relative, found := strings.CutPrefix(path, strings.TrimSuffix(home, "/")+"/"); found && home != "" {
		return "~/" + relative
	}
	return path
}

// ExpandHomeIn is ExpandHome with a given home directory.
func ExpandHomeIn(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// expandHomeInMap returns a copy, so the defaults' map is never changed.
func expandHomeInMap(pathsByName map[string][]string, home string) map[string][]string {
	expanded := make(map[string][]string, len(pathsByName))
	for name, paths := range pathsByName {
		expanded[name] = expandHomeAll(paths, home)
	}
	return expanded
}

func expandHomeAll(paths []string, home string) []string {
	expanded := make([]string, len(paths))
	for i, path := range paths {
		expanded[i] = ExpandHomeIn(path, home)
	}
	return expanded
}
