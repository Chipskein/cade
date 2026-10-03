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

func TestLoadMigratesPreviousGenerationDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"generation":{"model_path":"~/.local/share/cade/models/qwen2.5-3b-instruct-q4_k_m.gguf"}}`
	testcheck.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	cfg, err := Load(path)
	if err != nil || cfg.Generation.ModelPath != ExpandHome(defaultGenerationModelPath) {
		t.Fatalf("expected migrated generation model path, got %q (err %v)", cfg.Generation.ModelPath, err)
	}
}

func TestLoadKeepsCustomGenerationModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"generation":{"model_path":"~/models/custom.gguf"}}`
	testcheck.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	cfg, err := Load(path)
	if err != nil || cfg.Generation.ModelPath != ExpandHome("~/models/custom.gguf") {
		t.Fatalf("expected custom generation model path, got %q (err %v)", cfg.Generation.ModelPath, err)
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

func TestDefaultsLeaveImagesOffWithUsableLimits(t *testing.T) {
	defaults := Defaults()
	if defaults.Sources.Images || defaults.Ingest.validateImageLimits() != nil || defaults.Vision.ContextTokens <= 0 {
		t.Fatalf("expected images off by default with valid limits, got sources.images=%v ingest=%+v vision=%+v",
			defaults.Sources.Images, defaults.Ingest, defaults.Vision)
	}
}

func TestLoadRejectsImagesWithoutAPerRunLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"sources": {"images": true}, "ingest": {"max_images_per_run": 0}}`), 0o600))
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "max_images_per_run is 0") {
		t.Fatalf("expected max_images_per_run 0 rejected, got %v", err)
	}
}

// With images off, the limits are never read, so a zero is harmless.
func TestLoadIgnoresImageLimitsWithImagesOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ingest": {"max_image_bytes": 0}}`), 0o600))
	if _, err := Load(path); err != nil {
		t.Fatalf("expected image limits ignored with sources.images off, got %v", err)
	}
}

func TestLoadExpandsProjectorPath(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil || strings.HasPrefix(cfg.Vision.ProjectorPath, "~") || !strings.HasSuffix(cfg.Vision.ProjectorPath, "mmproj-Qwen3.5-2B-F16.gguf") {
		t.Fatalf("expected an expanded projector path, got %q (err %v)", cfg.Vision.ProjectorPath, err)
	}
}

func TestLoadExpandsIndexedDBSchemaPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"sources": {"indexeddb_dirs": {"whatsapp": ["~/.floorp/p/storage/default/https+++web.whatsapp.com/idb"]}}}`), 0o600))
	cfg, err := Load(path)
	home, _ := os.UserHomeDir()
	if err != nil || cfg.Sources.IndexedDBSchemaDir != filepath.Join(home, ".config/cade/idb-schemas") {
		t.Fatalf("expected the default schema directory expanded, got %q (err %v)", cfg.Sources.IndexedDBSchemaDir, err)
	}
	if got := cfg.Sources.IndexedDBDirs["whatsapp"]; len(got) != 1 || !strings.HasPrefix(got[0], home+"/.floorp/") {
		t.Fatalf("expected the whatsapp directory expanded, got %v", got)
	}
}

func TestDefaultsIndexedDBDirsStayEmptyAfterLoad(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Fatal(err)
	}
	if len(Defaults().Sources.IndexedDBDirs) != 0 {
		t.Fatal("loading must not change the defaults")
	}
}
