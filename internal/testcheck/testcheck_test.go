package testcheck

import (
	"errors"
	"testing"
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
