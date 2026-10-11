package devtasks

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadSettingsDefaultsUnderHome(t *testing.T) {
	settings := LoadSettings(FakeEnvironment{EnvHome: "/home/ana", EnvCudaHome: t.TempDir()}, "linux")
	if settings.ModelsDir != filepath.FromSlash("/home/ana/.local/share/cade/models") || settings.Prefix != filepath.FromSlash("/home/ana/.local") {
		t.Errorf("ModelsDir %q, Prefix %q; want them under /home/ana", settings.ModelsDir, settings.Prefix)
	}
	if settings.LlamaNative != "ON" || settings.EvalTimeout != "1h" || settings.FuzzTime != "30s" {
		t.Errorf("defaults = %+v", settings)
	}
	if got := settings.ModelPath(EmbeddingModel); got != filepath.FromSlash("/home/ana/.local/share/cade/models/nomic-embed-text-v2-moe.Q4_K_M.gguf") {
		t.Errorf("embedding model path = %q", got)
	}
}

func TestLoadSettingsVariablesOverrideDefaults(t *testing.T) {
	settings := LoadSettings(FakeEnvironment{EnvLlamaNative: "OFF", EnvModelsDir: "/models", EnvGenerationModel: "/other.gguf", EnvGoTags: "cuda, extra"}, "linux")
	if settings.LlamaNative != "OFF" || settings.ModelPath(EmbeddingModel) != filepath.Join("/models", "nomic-embed-text-v2-moe.Q4_K_M.gguf") {
		t.Errorf("settings = %+v", settings)
	}
	if settings.ModelPath(GenerationModel) != "/other.gguf" {
		t.Errorf("generation model = %q, want the GENERATION_MODEL override", settings.ModelPath(GenerationModel))
	}
	if !slices.Equal(settings.ExtraGoTags, []string{"cuda", "extra"}) {
		t.Errorf("ExtraGoTags = %q", settings.ExtraGoTags)
	}
}

func TestExtraGoTagsDetectsCUDAUnlessEmptyGoTags(t *testing.T) {
	world := newTestWorld(t)
	world.writeFile(t, "cuda/bin/nvcc", "")
	cudaHome := filepath.Join(world.root, "cuda")
	if got := LoadSettings(FakeEnvironment{EnvCudaHome: cudaHome}, "linux").ExtraGoTags; !slices.Equal(got, []string{"cuda"}) {
		t.Errorf("with nvcc installed: %q, want [cuda]", got)
	}
	if got := LoadSettings(FakeEnvironment{EnvCudaHome: cudaHome, EnvGoTags: ""}, "linux").ExtraGoTags; got != nil {
		t.Errorf("with GO_TAGS empty: %q, want none", got)
	}
	if got := LoadSettings(FakeEnvironment{EnvCudaHome: t.TempDir()}, "linux").ExtraGoTags; got != nil {
		t.Errorf("without nvcc: %q, want none", got)
	}
}

func TestNvccPathIsUnderCudaHome(t *testing.T) {
	if got := LoadSettings(FakeEnvironment{EnvCudaHome: "/opt/cuda"}, "linux").NvccPath(); got != filepath.FromSlash("/opt/cuda/bin/nvcc") {
		t.Errorf("NvccPath = %q", got)
	}
}

// On Windows cade looks for the models in AppData (#54).
func TestLoadSettingsPutsTheModelsInAppDataOnWindows(t *testing.T) {
	settings := LoadSettings(FakeEnvironment{EnvHome: "/home/ana"}, "windows")
	if want := filepath.FromSlash("/home/ana/AppData/Local/cade/models"); settings.ModelsDir != want {
		t.Fatalf("ModelsDir = %q, want %q", settings.ModelsDir, want)
	}
}
