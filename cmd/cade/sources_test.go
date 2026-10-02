package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/ingest"
)

func TestSourceSpecsRegistersAllSources(t *testing.T) {
	names := ingest.SourceNames(sourceSpecs(config.Defaults(), nil))
	if len(names) != 4 || names[0] != "git" || names[1] != "browser" || names[2] != "file" || names[3] != "teams" {
		t.Fatalf("expected [git browser file teams], got %v", names)
	}
}

func TestSourceSpecsAddsOneSourcePerSavedSchema(t *testing.T) {
	cfg := config.Defaults()
	cfg.Sources.IndexedDBSchemaDir = t.TempDir()
	cfg.Sources.IndexedDBDirs = map[string][]string{"teams-web": {"../../testdata/teams-formats/2026-09.leveldb"}}
	copySchema(t, "teams-web", cfg.Sources.IndexedDBSchemaDir)
	writeFile(t, filepath.Join(cfg.Sources.IndexedDBSchemaDir, "teams.json"), "{}")
	specs := sourceSpecs(cfg, nil)
	names := ingest.SourceNames(specs)
	if len(names) != 5 || names[4] != "teams-web" || specs[4].DefaultTargets[0] != cfg.Sources.IndexedDBDirs["teams-web"][0] {
		t.Fatalf("expected the built-in sources then teams-web with its directory, got %v", names)
	}
	collector, err := specs[4].NewCollector(specs[4].DefaultTargets[0])
	if err != nil || collector == nil {
		t.Fatalf("expected a collector from the saved schema, got %v", err)
	}
}

func TestSchemaCollectorReportsABrokenSchemaFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "zap.json"), `{"version": 1`)
	specs := schemaSpecs(config.Defaults().Sources, idbmap.NewSchemaDir(idbmap.OSSchemaFiles{}, dir), nil)
	if len(specs) != 1 {
		t.Fatalf("expected the broken schema registered, got %d specs", len(specs))
	}
	if _, err := specs[0].NewCollector("/x"); err == nil || !strings.Contains(err.Error(), "zap.json") {
		t.Fatalf("expected the error to name the file, got %v", err)
	}
}

func copySchema(t *testing.T, name, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../testdata/idb-schemas", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, name+".json"), string(raw))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
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
