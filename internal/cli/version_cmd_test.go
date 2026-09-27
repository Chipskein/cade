package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/buildinfo"
)

func versionWorld() *fakeWorld {
	world := newFakeWorld()
	world.build = buildinfo.Info{Version: "v0.0.0", Commit: "8727192", Date: "2026-09-27", Accelerator: "CPU", LlamaTag: "b11195"}
	return world
}

func TestVersionCommandAndFlag(t *testing.T) {
	want := "cade v0.0.0 (commit 8727192, 2026-09-27, CPU build, llama.cpp b11195)\n"
	for _, args := range [][]string{{"version"}, {"--version"}, {"-version"}} {
		code, stdout, stderr := versionWorld().run(args...)
		if code != 0 || stdout != want {
			t.Errorf("%v: expected %q, got %d %q %q", args, want, code, stdout, stderr)
		}
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	if code, _, stderr := versionWorld().run("version", "extra"); code != 1 || !strings.Contains(stderr, "extra") {
		t.Fatalf("expected exit 1, got %d %q", code, stderr)
	}
}
