package testcheck

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/privatefile"
)

// FatalRecorder records Fatalf instead of stopping the running test.
type FatalRecorder struct {
	testing.TB
	Message string
}

func (r *FatalRecorder) Helper() {}

func (r *FatalRecorder) Fatalf(format string, args ...any) {
	r.Message = format
}

func TestNoErrorPassesNil(t *testing.T) {
	recorder := &FatalRecorder{TB: t}
	NoError(recorder, nil)
	if recorder.Message != "" {
		t.Fatalf("expected no failure for a nil error, got %q", recorder.Message)
	}
}

func TestNoErrorFailsOnError(t *testing.T) {
	recorder := &FatalRecorder{TB: t}
	NoError(recorder, errors.New("disk full"))
	if recorder.Message == "" {
		t.Fatal("expected a failure for a non-nil error")
	}
}

func TestOwnerOnlyAcceptsAnOwnerOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	NoError(t, os.WriteFile(path, nil, 0o600))
	OwnerOnly(t, path)
}

func TestOwnerOnlyFailsOnAFileOthersReadWhereModesExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared")
	NoError(t, os.WriteFile(path, nil, 0o600))
	NoError(t, os.Chmod(path, 0o644))
	recorder := &FatalRecorder{TB: t}
	OwnerOnly(recorder, path)
	if failed := recorder.Message != ""; failed != privatefile.ModesEnforced {
		t.Fatalf("expected a failure only where modes are enforced, got %q", recorder.Message)
	}
}
