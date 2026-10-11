package devtasks

import (
	"slices"
	"testing"
)

func TestCgoEnvAddsTheLocalPrefixOnFreeBSDOnly(t *testing.T) {
	if got := cgoEnv("freebsd", FakeEnvironment{}); !slices.Equal(got, []string{"CGO_CFLAGS=-I/usr/local/include -D__COSMOPOLITAN__", "CGO_LDFLAGS=-L/usr/local/lib"}) {
		t.Errorf("freebsd: got %q", got)
	}
	if got := cgoEnv("linux", FakeEnvironment{}); got != nil {
		t.Errorf("linux: expected nothing, got %q", got)
	}
	kept := cgoEnv("freebsd", FakeEnvironment{EnvCgoCFlags: "-O2"})
	if kept[0] != "CGO_CFLAGS=-O2 -I/usr/local/include -D__COSMOPOLITAN__" {
		t.Errorf("expected the user's flags kept first, got %q", kept)
	}
}

func TestCgoRunnerGivesTheFlagsToGoCommandsOnly(t *testing.T) {
	inner := &FakeCommandRunner{Outputs: map[string]string{}}
	runner := withCgoEnv(inner, "freebsd", FakeEnvironment{})
	_ = runner.Run(Command{Name: "go", Args: []string{"build"}, Env: []string{"K=v"}})
	_ = runner.Run(Command{Name: "cmake", Args: []string{"--build"}})
	if got := inner.Commands[0].Env; !slices.Equal(got, []string{"CGO_CFLAGS=-I/usr/local/include -D__COSMOPOLITAN__", "CGO_LDFLAGS=-L/usr/local/lib", "K=v"}) {
		t.Errorf("go got env %q", got)
	}
	if got := inner.Commands[1].Env; len(got) != 0 {
		t.Errorf("cmake got env %q", got)
	}
	if withCgoEnv(inner, "linux", FakeEnvironment{}) != CommandRunner(inner) {
		t.Error("expected the runner unwrapped on Linux")
	}
}
