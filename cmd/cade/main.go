// Command cade aggregates local activity (git, browser, files) and answers
// timeline and natural-language questions about it, entirely offline.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chipskein/cade/internal/cli"
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, productionToolkit())
	stop()
	os.Exit(code)
}

func productionToolkit() cli.Toolkit {
	return cli.Toolkit{
		DefaultConfigPath: config.DefaultPath,
		LoadConfig:        config.Load,
		WriteConfig:       config.WriteDefault,
		OpenStore:         openStore,
		LoadEmbedder:      loadEmbedder,
		LoadGenerator:     loadGenerator,
		Sources:           sourceSpecs,
		ReadIndexedDB:     indexeddb.ReadDirectory,
		StderrIsTerminal:  isTerminal(os.Stderr),
		Now:               time.Now,
	}
}

func openStore(ctx context.Context, path string) (storage.EventStore, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory for %q: %w", path, err)
	}
	// MkdirAll keeps the mode of an existing directory (`make models`
	// creates it first, world-readable); the history must be owner-only.
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("restrict database directory %q to 700: %w", directory, err)
	}
	return sqlitestore.OpenWithHooks(ctx, path, sqlitestore.Hooks{BackupCreated: func(backupPath string) {
		fmt.Fprintf(os.Stderr, "Banco atualizado para o novo esquema; cópia da versão anterior em %s\n", backupPath)
	}})
}

func loadEmbedder(settings config.EmbeddingConfig) (cli.ClosableEmbedder, error) {
	return llamacpp.LoadEmbedder(modelOptions(settings.ModelConfig))
}

func loadGenerator(settings config.ModelConfig) (cli.ClosableGenerator, error) {
	return llamacpp.LoadGenerator(modelOptions(settings))
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

// openSQLiteFile opens a foreign SQLite file (browser history) with the
// same driver the store registers.
func openSQLiteFile(path string) (*sql.DB, error) {
	return sql.Open(sqlitestore.DriverName, "file:"+path)
}
