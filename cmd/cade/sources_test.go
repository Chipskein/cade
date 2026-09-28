package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/ingest"
)

func TestSourceSpecsRegistersAllSources(t *testing.T) {
	names := ingest.SourceNames(sourceSpecs(config.Defaults(), nil))
	if len(names) != 4 || names[0] != "git" || names[1] != "browser" || names[2] != "file" || names[3] != "teams" {
		t.Fatalf("expected [git browser file teams], got %v", names)
	}
}

func TestIsTerminalFalseForRegularFile(t *testing.T) {
	file, _ := os.CreateTemp(t.TempDir(), "out")
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("a regular file is not a terminal")
	}
}

func TestNewTeamsCollector(t *testing.T) {
	if collector, err := newTeamsCollector(t.TempDir()); err != nil || collector == nil {
		t.Fatalf("expected a collector, got %v", err)
	}
}

func TestFileCollectorRejectsMissingDirectory(t *testing.T) {
	factory := fileCollectorFactory(config.Defaults().Sources, nil)
	if _, err := factory(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestFileCollectorAcceptsDirectory(t *testing.T) {
	factory := fileCollectorFactory(config.Defaults().Sources, nil)
	if _, err := factory(t.TempDir()); err != nil {
		t.Fatalf("expected a collector, got %v", err)
	}
}

func TestModelOptionsCopiesSettings(t *testing.T) {
	opts := modelOptions(config.ModelConfig{ModelPath: "/m.gguf", ContextTokens: 4096, Threads: 4, GPULayers: 99})
	if opts.Path != "/m.gguf" || opts.ContextTokens != 4096 || opts.Threads != 4 || opts.GPULayers != 99 {
		t.Fatalf("unexpected options %+v", opts)
	}
}

func TestOpenStoreCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cade.db")
	store, err := openStore(t.Context(), path, nil)
	if err != nil {
		t.Fatalf("expected store, got %v", err)
	}
	store.Close()
}
