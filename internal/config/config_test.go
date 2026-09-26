package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsExpandedDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	home, _ := os.UserHomeDir()
	if err != nil || cfg.DatabasePath != filepath.Join(home, ".local/share/cade/cade.db") {
		t.Fatalf("expected expanded default database path, got %q (err %v)", cfg.DatabasePath, err)
	}
}

func TestLoadOverridesOnlyGivenFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"retrieval": {"top_k": 3}, "sources": {"directories": ["/notes"]}}`), 0o600)
	cfg, err := Load(path)
	if err != nil || cfg.Retrieval.TopK != 3 || cfg.Retrieval.MaxAnswerTokens != 512 || cfg.Sources.Directories[0] != "/notes" {
		t.Fatalf("expected override merged over defaults, got %+v (err %v)", cfg, err)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{not json`), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestWriteDefaultThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	if err := WriteDefault(path); err != nil {
		t.Fatalf("write default: %v", err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.Retrieval.TopK != Defaults().Retrieval.TopK {
		t.Fatalf("expected defaults to round-trip, got %+v (err %v)", cfg, err)
	}
}

func TestWriteDefaultRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{}`), 0o600)
	if err := WriteDefault(path); err == nil {
		t.Fatal("expected refusal to overwrite an existing config")
	}
}

func TestExpandHome(t *testing.T) {
	cases := map[string]string{"~": "/h", "~/x/y": "/h/x/y", "/abs": "/abs", "rel/~": "rel/~"}
	for input, expected := range cases {
		if got := expandHome(input, "/h"); got != expected {
			t.Errorf("expandHome(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestExpandHomeAll(t *testing.T) {
	got := expandHomeAll([]string{"~/a", "/b"}, "/h")
	if got[0] != "/h/a" || got[1] != "/b" {
		t.Fatalf("expected [/h/a /b], got %v", got)
	}
}

func TestDefaultPathEndsWithCadeConfig(t *testing.T) {
	path, err := DefaultPath()
	if err != nil || filepath.Base(path) != "config.json" || filepath.Base(filepath.Dir(path)) != "cade" {
		t.Fatalf("expected .../cade/config.json, got %q (err %v)", path, err)
	}
}

func TestDefaultsOffloadAllLayers(t *testing.T) {
	cfg := Defaults()
	if cfg.Generation.GPULayers != -1 || cfg.Embedding.GPULayers != -1 {
		t.Fatalf("expected all layers offloaded by default, got %d / %d", cfg.Generation.GPULayers, cfg.Embedding.GPULayers)
	}
}
