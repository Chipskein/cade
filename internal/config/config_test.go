package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
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
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"retrieval": {"top_k": 3}, "sources": {"directories": ["/notes"]}}`), 0o600))
	cfg, err := Load(path)
	if err != nil || cfg.Retrieval.TopK != 3 || cfg.Retrieval.MaxAnswerTokens != 512 || cfg.Sources.Directories[0] != "/notes" {
		t.Fatalf("expected override merged over defaults, got %+v (err %v)", cfg, err)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{not json`), 0o600))
	if _, err := Load(path); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestWriteThenLoadOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	cfg := Defaults()
	cfg.Sources.Directories = []string{"/notes"}
	if err := Write(path, cfg); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Retrieval.TopK != Defaults().Retrieval.TopK || loaded.Sources.Directories[0] != "/notes" {
		t.Fatalf("expected the config to round-trip, got %+v (err %v)", loaded, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 600, got %v (err %v)", info.Mode(), err)
	}
}

func TestWriteRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	if err := Write(path, Defaults()); err == nil {
		t.Fatal("expected refusal to overwrite an existing config")
	}
	if raw, _ := os.ReadFile(path); string(raw) != `{}` {
		t.Fatalf("the existing config changed: %q", raw)
	}
}

func TestContractHome(t *testing.T) {
	cases := map[string]string{"/h": "~", "/h/src/app": "~/src/app", "/hx/app": "/hx/app", "/etc/x": "/etc/x"}
	for input, expected := range cases {
		if got := ContractHome(input, "/h"); got != expected {
			t.Errorf("ContractHome(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestContractHomeWithoutHome(t *testing.T) {
	if got := ContractHome("/src/app", ""); got != "/src/app" {
		t.Fatalf("expected the path unchanged, got %q", got)
	}
}

func TestExpandHomeIn(t *testing.T) {
	cases := map[string]string{"~": "/h", "~/x/y": "/h/x/y", "/abs": "/abs", "rel/~": "rel/~"}
	for input, expected := range cases {
		if got := ExpandHomeIn(input, "/h"); got != expected {
			t.Errorf("ExpandHomeIn(%q) = %q, expected %q", input, got, expected)
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

func TestDefaultTaskPatternsCompile(t *testing.T) {
	for _, pattern := range Defaults().Tasks.TaskURLPatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			t.Errorf("default task pattern %q does not compile: %v", pattern, err)
		}
	}
}

func TestLoadRejectsUnknownUILanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ui": {"language": "fr"}}`), 0o600))
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), `"fr"`) {
		t.Fatalf("expected ui.language \"fr\" rejected, got %v", err)
	}
}

func TestLoadAcceptsUILanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ui": {"language": "en"}}`), 0o600))
	if cfg, err := Load(path); err != nil || cfg.UI.Language != "en" {
		t.Fatalf("expected en, got %+v %v", cfg.UI, err)
	}
}

func TestLoadRejectsUnknownDateOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ui": {"date_order": "ymd"}}`), 0o600))
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "ymd") {
		t.Fatalf("expected ui.date_order \"ymd\" rejected, got %v", err)
	}
}

func TestLoadDefaultsDateOrderToAuto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ui": {"language": "en"}}`), 0o600))
	if cfg, err := Load(path); err != nil || cfg.UI.DateOrder != "auto" {
		t.Fatalf("expected ui.date_order \"auto\" by default, got %q (%v)", cfg.UI.DateOrder, err)
	}
}
