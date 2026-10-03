// Command cade aggregates local activity (git, browser, files) and answers
// timeline and natural-language questions about it, entirely offline.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chipskein/cade/internal/buildinfo"
	"github.com/chipskein/cade/internal/cli"
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/imagepreview"
	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/pacing"
	"github.com/chipskein/cade/internal/procctl"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
)

func main() {
	// SIGHUP (the terminal closed) cancels like Ctrl-C, so a foreground
	// ingest records that it was interrupted and can be resumed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, productionToolkit())
	stop()
	os.Exit(code)
}

func productionToolkit() cli.Toolkit {
	return cli.Toolkit{
		DefaultConfigPath:  config.DefaultPath,
		LoadConfig:         config.Load,
		WriteConfig:        config.Write,
		OpenStore:          openStore,
		InspectDatabase:    sqlitestore.Inspect,
		RootFS:             os.DirFS("/"),
		HomeDir:            os.UserHomeDir,
		Stdin:              os.Stdin,
		Build:              buildinfo.Read(),
		LoadEmbedder:       loadEmbedder,
		LoadGenerator:      loadGenerator,
		LoadImageDescriber: loadImageDescriber,
		Sources:            sourceSpecs,
		StoreReaders:       storeReaders(),
		SchemaFiles:        idbmap.OSSchemaFiles{},
		StderrIsTerminal:   isTerminal(os.Stderr),
		StdoutIsTerminal:   isTerminal(os.Stdout),
		RenderImagePreview: renderImagePreview,
		Language:           cli.LanguageFromEnv(os.Getenv),
		DateOrder:          cli.DateOrderFromEnv(os.Getenv),
		Now:                time.Now,
		IngestRuns:         ingestRunTools(),
	}
}

func ingestRunTools() cli.IngestRunTools {
	dir := ingestRunDir()
	return cli.IngestRunTools{
		State:     ingestrun.JSONStateFile{Dir: dir},
		Lock:      ingestrun.FileLock{Dir: dir},
		LogPath:   dir.LogPath(),
		Processes: procctl.System{},
		Clock:     pacing.SystemClock{},
		Detached:  os.Getenv(cli.DetachedIngestVariable) == "1",
		PID:       os.Getpid(),
	}
}

// ingestRunDir falls back to the temporary directory without a home, where
// the database path cannot be expanded either and ingest fails first.
func ingestRunDir() ingestrun.Dir {
	dir, err := ingestrun.DefaultDir(os.Getenv, os.UserHomeDir)
	if err != nil {
		return ingestrun.Dir(filepath.Join(os.TempDir(), "cade"))
	}
	return dir
}

func renderImagePreview(path string) string {
	previewer := imagepreview.Previewer{Runner: imagepreview.ExecRunner{}, LookPath: exec.LookPath, Size: imagepreview.DefaultPreviewSize}
	return previewer.Render(context.Background(), path)
}

func openStore(ctx context.Context, path string, backupCreated func(backupPath string)) (storage.EventStore, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory for %q: %w", path, err)
	}
	// MkdirAll keeps the mode of an existing directory (`go tool mage models`
	// creates it first, world-readable); the history must be owner-only.
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("restrict database directory %q to 700: %w", directory, err)
	}
	return sqlitestore.OpenWithHooks(ctx, path, sqlitestore.Hooks{BackupCreated: backupCreated})
}

func loadEmbedder(settings config.EmbeddingConfig, logger *slog.Logger) (cli.ClosableEmbedder, error) {
	opts := modelOptions(settings.ModelConfig)
	opts.Logger = logger
	return llamacpp.LoadEmbedder(opts)
}

func loadGenerator(settings config.ModelConfig, logger *slog.Logger) (cli.ClosableGenerator, error) {
	opts := modelOptions(settings)
	opts.Logger, opts.PromptStateDir = logger, promptStateDir()
	return llamacpp.LoadGenerator(opts)
}

// loadImageDescriber runs the generation model with the smaller context
// of vision.context_tokens, and no saved prompt state: each image differs.
func loadImageDescriber(generation config.ModelConfig, vision config.VisionConfig, logger *slog.Logger) (imagecaption.ClosableDescriber, error) {
	opts := modelOptions(generation)
	opts.ContextTokens, opts.Logger = vision.ContextTokens, logger
	return llamacpp.LoadImageDescriber(opts, vision.ProjectorPath)
}

// promptStateDir holds the question planner's saved prompt state (~70 MB,
// instructions and examples only, no history). Without a cache directory
// the planner just decodes its prompt every time.
func promptStateDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "cade", "prompt-state")
}

func modelOptions(settings config.ModelConfig) llamacpp.ModelOptions {
	return llamacpp.ModelOptions{
		Path:          settings.ModelPath,
		ContextTokens: settings.ContextTokens,
		Threads:       settings.Threads,
		GPULayers:     settings.GPULayers,
	}
}

// isTerminal reports whether file is a character device (a TTY), so the
// CLI knows it may redraw a progress line in place.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// sqliteURIEscaper escapes what a file: URI would otherwise read as an
// escape, the query or the fragment: Firefox names IndexedDB files like
// "2838935879%e2gFawrao_tvso_ip.sqlite".
var sqliteURIEscaper = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")

// openSQLiteFile opens a foreign SQLite file (a snapshot of a browser
// history or IndexedDB) with the same driver the store registers.
func openSQLiteFile(path string) (*sql.DB, error) {
	return sql.Open(sqlitestore.DriverName, "file:"+sqliteURIEscaper.Replace(path))
}
