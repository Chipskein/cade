package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

func TestDefaultBackgroundLimitsAreValid(t *testing.T) {
	if err := defaultBackground().validate(true); err != nil {
		t.Fatalf("expected the default background limits valid with images on, got %v", err)
	}
}

func TestWithBackgroundLimitsReplacesModelSettings(t *testing.T) {
	cfg := Defaults()
	cfg.Ingest.Background = BackgroundConfig{Threads: 3, GPULayers: 0, BusyPercent: 40, MaxImagesPerRun: 900}
	limited := cfg.WithBackgroundLimits()
	if limited.Embedding.Threads != 3 || limited.Generation.Threads != 3 || limited.Embedding.GPULayers != 0 ||
		limited.Generation.GPULayers != 0 || limited.Ingest.MaxImagesPerRun != 900 {
		t.Fatalf("expected threads 3, gpu_layers 0 and 900 images per run, got embedding %+v generation %+v ingest %+v",
			limited.Embedding.ModelConfig, limited.Generation, limited.Ingest)
	}
	if cfg.Embedding.Threads != 0 {
		t.Fatalf("expected the original config untouched, got embedding threads %d", cfg.Embedding.Threads)
	}
}

func TestLoadRejectsBusyPercentOutOfRange(t *testing.T) {
	for _, busy := range []string{"0", "101"} {
		path := filepath.Join(t.TempDir(), "config.json")
		testcheck.NoError(t, os.WriteFile(path, []byte(`{"ingest": {"background": {"busy_percent": `+busy+`}}}`), 0o600))
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "busy_percent is "+busy) {
			t.Fatalf("expected busy_percent %s rejected, got %v", busy, err)
		}
	}
}

func TestLoadRejectsNegativeBackgroundThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	testcheck.NoError(t, os.WriteFile(path, []byte(`{"ingest": {"background": {"threads": -1}}}`), 0o600))
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "threads is -1") {
		t.Fatalf("expected negative threads rejected, got %v", err)
	}
}

func TestLoadRejectsNoBackgroundImagesWithImagesOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"sources": {"images": true}, "ingest": {"background": {"max_images_per_run": 0}}}`
	testcheck.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "background.max_images_per_run is 0") {
		t.Fatalf("expected background.max_images_per_run 0 rejected, got %v", err)
	}
}
