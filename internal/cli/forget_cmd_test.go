package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestForgetMatchDeletesAndReportsTheCount(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{
		{UID: "a", Source: event.SourceGit, Timestamp: cliNow, Content: "café da manhã"},
		{UID: "b", Source: event.SourceGit, Timestamp: cliNow, Content: "café da tarde"},
	}
	code, stdout, stderr := world.run("forget", "--match", "café", "--yes")
	if code != 0 {
		t.Fatalf("expected success, got %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "2 eventos removidos.") {
		t.Fatalf("expected the removed count in stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "Apagando eventos: 2/2 (100%)") {
		t.Fatalf("expected a progress line with the done/total, got %q", stderr)
	}
	if len(world.store.Events) != 0 || !world.store.Forgotten["a"] || !world.store.Forgotten["b"] {
		t.Fatalf("expected both matches deleted, got %+v %+v", world.store.Events, world.store.Forgotten)
	}
}

func TestForgetMatchWithNoMatchesDeletesNothing(t *testing.T) {
	world := newFakeWorld()
	code, stdout, stderr := world.run("forget", "--match", "café", "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("expected a quiet success with nothing to delete, got %d %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("expected no summary when there was nothing to delete, got %q", stdout)
	}
}
