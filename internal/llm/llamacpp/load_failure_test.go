package llamacpp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFailureTellsAMissingFileFromAnUnloadableOne(t *testing.T) {
	missing := loadFailure(ModelOptions{Path: filepath.Join(t.TempDir(), "absent.gguf"), GPULayers: -1})
	if !errors.Is(missing, fs.ErrNotExist) {
		t.Errorf("expected a missing file reported as such, got %v", missing)
	}
	present := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(present, []byte("GGUF"), 0o600); err != nil {
		t.Fatal(err)
	}
	unloadable := loadFailure(ModelOptions{Path: present, GPULayers: -1}).Error()
	if !strings.Contains(unloadable, "gpu_layers -1") || !strings.Contains(unloadable, "not enough memory") {
		t.Errorf("expected the memory hint with the layers asked, got %q", unloadable)
	}
}
