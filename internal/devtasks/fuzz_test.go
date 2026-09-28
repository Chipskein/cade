package devtasks

import "testing"

func TestFuzzRunsEveryTarget(t *testing.T) {
	world := newTestWorld(t)
	world.env[EnvFuzzTime] = "5s"
	if err := world.tasks().Fuzz(); err != nil {
		t.Fatal(err)
	}
	if len(world.runner.Commands) != len(fuzzTargets) {
		t.Fatalf("ran %d targets, want %d", len(world.runner.Commands), len(fuzzTargets))
	}
	assertLines(t, world.runner.Lines()[:1], "go test -tags sqlite_fts5 -run ^$ -fuzz ^FuzzJournalBatches$ -fuzztime 5s ./internal/leveldbraw")
}

func TestFuzzStopsAtTheFirstFailure(t *testing.T) {
	world := newTestWorld(t)
	world.runner.FailOn = "FuzzDecodeBatch"
	if err := world.tasks().Fuzz(); err == nil || len(world.runner.Commands) != 2 {
		t.Errorf("err %v after %d commands; want a failure at the second", err, len(world.runner.Commands))
	}
	if world.progress.String() != "== leveldbraw FuzzJournalBatches\n== leveldbraw FuzzDecodeBatch\n" {
		t.Errorf("progress = %q", world.progress.String())
	}
}
