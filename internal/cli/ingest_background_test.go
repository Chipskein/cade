package cli

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingestrun"
)

func TestIngestRecordsAFinishedRun(t *testing.T) {
	world := newFakeWorld()
	code, _, stderr := world.run("ingest", "git")
	state := world.runState.State
	if code != 0 || state.Status != ingestrun.StatusFinished || !slices.Equal(state.Args, []string{"git"}) || state.Job != "git /repo" ||
		state.JobNumber != 1 || state.JobCount != 1 || state.PID != 1234 || state.LogPath != "" {
		t.Fatalf("expected a finished run of `git` recorded, got %d %+v (%q)", code, state, stderr)
	}
	if world.runLock.Acquired != 1 || world.runLock.Released != 1 {
		t.Fatalf("expected the lock taken and released once, got %+v", world.runLock)
	}
}

func TestIngestRecordsAFailedRunWithItsError(t *testing.T) {
	world := newFakeWorld()
	world.store.FailWith = errors.New("disk full")
	world.run("ingest", "git")
	if state := world.runState.State; state.Status != ingestrun.StatusFailed || !strings.Contains(state.Error, "disk full") {
		t.Fatalf("expected a failed run with the error, got %+v", state)
	}
}

func TestIngestRefusesWhileAnotherRuns(t *testing.T) {
	world := newFakeWorld()
	world.runLock.Held = true
	world.runState.State, world.runState.Found = ingestrun.State{PID: 99, Status: ingestrun.StatusRunning}, true
	code, _, stderr := world.run("ingest", "git")
	if code != 1 || !strings.Contains(stderr, "pid 99") || world.embedderLoads != 0 || len(world.runState.Writes) != 0 {
		t.Fatalf("expected a refusal naming pid 99 before any model loads, got %d %q", code, stderr)
	}
}

func TestIngestTypoKeepsTheLastRunState(t *testing.T) {
	world := newFakeWorld()
	if code, _, _ := world.run("ingest", "nope"); code != 1 || len(world.runState.Writes) != 0 || world.runLock.Acquired != 0 {
		t.Fatalf("expected an unknown source to fail before recording, got %d with %d writes", code, len(world.runState.Writes))
	}
}

func TestGentleIngestUsesTheBackgroundLimits(t *testing.T) {
	world := newFakeWorld()
	world.pacingClock.Step = time.Second
	code, _, stderr := world.run("ingest", "--gentle", "git")
	if code != 0 || world.loadedEmbedding.Threads != world.cfg.Ingest.Background.Threads || !world.processes.PriorityLowered {
		t.Fatalf("expected background threads and lowered priority, got %d threads %d (%q)", code, world.loadedEmbedding.Threads, stderr)
	}
	if len(world.pacingClock.Slept) == 0 || !world.runState.State.Mode.Gentle || world.runState.State.Mode.Background {
		t.Fatalf("expected the embedder paced in a gentle foreground run, got rests %v, mode %+v", world.pacingClock.Slept, world.runState.State.Mode)
	}
}

func TestRegularIngestIsNeitherLimitedNorPaced(t *testing.T) {
	world := newFakeWorld()
	world.pacingClock.Step = time.Second
	world.run("ingest", "git")
	if world.processes.PriorityLowered || len(world.pacingClock.Slept) != 0 || world.loadedEmbedding.Threads != world.cfg.Embedding.Threads {
		t.Fatalf("expected a regular run at full speed, got priority lowered %v, rests %v", world.processes.PriorityLowered, world.pacingClock.Slept)
	}
}

func TestGentleIngestWarnsWhenPriorityStays(t *testing.T) {
	world := newFakeWorld()
	world.language = English
	world.processes.PriorityError = errors.New("permission denied")
	code, _, stderr := world.run("ingest", "--gentle", "git")
	if code != 0 || !strings.Contains(stderr, "warning: priority not lowered: permission denied") {
		t.Fatalf("expected a warning and a finished run, got %d %q", code, stderr)
	}
}

func TestBackgroundIngestStartsADetachedCade(t *testing.T) {
	world := newFakeWorld()
	code, stdout, stderr := world.run("--config", "/c.json", "ingest", "start", "git")
	if code != 0 || len(world.processes.Started) != 1 || world.embedderLoads != 0 || !strings.Contains(stdout, "pid 4321") {
		t.Fatalf("expected a detached start and no model here, got %d %q %q", code, stdout, stderr)
	}
	started := world.processes.Started[0]
	if !slices.Equal(started.Args, []string{"--config", "/c.json", "ingest", "--", "git"}) || !slices.Equal(started.Env, []string{DetachedIngestVariable + "=1"}) ||
		started.LogPath != "/state/cade/ingest.log" || world.runLock.Held {
		t.Fatalf("expected cade re-run detached with the same config and a log, got %+v (lock held %v)", started, world.runLock.Held)
	}
}

func TestBackgroundIngestFailsHereOnUnknownSource(t *testing.T) {
	world := newFakeWorld()
	if code, _, _ := world.run("ingest", "start", "nope"); code != 1 || len(world.processes.Started) != 0 {
		t.Fatalf("expected the typo reported before detaching, got %d with %d started", code, len(world.processes.Started))
	}
}

func TestBackgroundIngestRefusesWhileAnotherRuns(t *testing.T) {
	world := newFakeWorld()
	world.runLock.Held = true
	if code, _, _ := world.run("ingest", "start", "git"); code != 1 || len(world.processes.Started) != 0 {
		t.Fatalf("expected no second background run, got %d with %d started", code, len(world.processes.Started))
	}
}

func TestDetachedProcessRunsGentleInTheBackground(t *testing.T) {
	world := newFakeWorld()
	world.detached = true
	world.run("--config", "/c.json", "ingest", "--", "git")
	state := world.runState.State
	if state.Mode != (ingestrun.Mode{Background: true, Gentle: true}) || state.LogPath != "/state/cade/ingest.log" || !world.processes.PriorityLowered {
		t.Fatalf("expected a gentle background run with its log recorded, got %+v", state)
	}
}

func TestIngestSubcommandsTakeNoArguments(t *testing.T) {
	for _, command := range []string{"status", "stop", "pause", "resume"} {
		if code, _, stderr := newFakeWorld().run("ingest", command, "git"); code != 1 || !strings.Contains(stderr, "cade ingest "+command+" não recebe argumentos") {
			t.Fatalf("expected `ingest %s git` rejected, got %d %q", command, code, stderr)
		}
	}
}

func TestIngestPauseFreezesTheRunAndRecordsIt(t *testing.T) {
	world := newFakeWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	world.processes.AlivePIDs = []int{77}
	code, stdout, _ := world.run("ingest", "pause")
	if code != 0 || !slices.Equal(world.processes.Paused, []int{77}) || world.runState.State.Status != ingestrun.StatusPaused ||
		!strings.Contains(stdout, "cade ingest resume") {
		t.Fatalf("expected pid 77 paused and recorded, got %d %v %+v %q", code, world.processes.Paused, world.runState.State, stdout)
	}
}

func TestIngestPauseOfAPausedRunChangesNothing(t *testing.T) {
	world := pausedWorld()
	code, stdout, _ := world.run("ingest", "pause")
	if code != 0 || len(world.processes.Paused) != 0 || !strings.Contains(stdout, "já está pausada") {
		t.Fatalf("expected nothing done for a paused run, got %d %v %q", code, world.processes.Paused, stdout)
	}
}

func TestIngestResumeContinuesAPausedRun(t *testing.T) {
	world := pausedWorld()
	code, _, _ := world.run("ingest", "resume")
	if code != 0 || !slices.Equal(world.processes.Continued, []int{77}) || world.runState.State.Status != ingestrun.StatusRunning ||
		world.embedderLoads != 0 || len(world.processes.Started) != 0 {
		t.Fatalf("expected pid 77 woken, not a new run, got %d %v %+v", code, world.processes.Continued, world.runState.State)
	}
}

func TestIngestRefusesToStartWhileAnotherIsPaused(t *testing.T) {
	world := pausedWorld()
	world.runLock.Held = true
	for _, args := range [][]string{{"ingest", "git"}, {"ingest", "start", "git"}} {
		if code, _, stderr := world.run(args...); code != 1 || !strings.Contains(stderr, "está pausada, não concluída") {
			t.Fatalf("expected %v refused while paused, got %d %q", args, code, stderr)
		}
	}
}

func TestIngestStatusOfAPausedRun(t *testing.T) {
	_, stdout, _ := pausedWorld().run("ingest", "status")
	if !strings.Contains(stdout, "Ingestão pausada (pid 77") || !strings.Contains(stdout, "cade ingest resume") || strings.Contains(stdout, "cade ingest pause") {
		t.Fatalf("expected a paused run with resume and stop hints, got:\n%s", stdout)
	}
}

// pausedWorld has a live background run, paused.
func pausedWorld() *fakeWorld {
	world := newFakeWorld()
	state := runningState(77)
	state.Status = ingestrun.StatusPaused
	world.runState.State, world.runState.Found = state, true
	world.processes.AlivePIDs = []int{77}
	return world
}

func TestIngestStatusWithoutAnyRun(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("ingest", "status")
	if code != 0 || stdout != "Nenhuma ingestão registrada ainda.\n" {
		t.Fatalf("expected no run reported, got %d %q", code, stdout)
	}
}

// runningState is a background run of `file /notes` started an hour ago.
func runningState(pid int) ingestrun.State {
	return ingestrun.State{PID: pid, Args: []string{"file", "/notes"}, Mode: ingestrun.Mode{Background: true, Gentle: true},
		Status: ingestrun.StatusRunning, Stage: ingestrun.StageIngesting, StartedAt: cliNow.Add(-time.Hour), UpdatedAt: cliNow.Add(-2 * time.Second),
		Job: "file /notes", JobNumber: 1, JobCount: 2, Progress: "file /notes: 120 lidos de 500 (24%) · ETA ~3m10s", LogPath: "/state/cade/ingest.log"}
}

func TestIngestStatusShowsTheLiveRun(t *testing.T) {
	world := newFakeWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	world.processes.AlivePIDs = []int{77}
	_, stdout, _ := world.run("ingest", "status")
	for _, want := range []string{"Ingestão em andamento (pid 77, em segundo plano, modo gentil)", "(1:00:00); atualizada há 0:02",
		"etapa 1 de 2: file /notes", "ETA ~3m10s", "cade ingest pause", "cade ingest stop", "/state/cade/ingest.log"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in the status, got:\n%s", want, stdout)
		}
	}
}

func TestIngestStatusOfAVanishedRunSuggestsResuming(t *testing.T) {
	world := newFakeWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	_, stdout, _ := world.run("ingest", "status")
	if !strings.Contains(stdout, "pid 77 não existe mais") || !strings.Contains(stdout, "cade ingest resume") || strings.Contains(stdout, "stop") {
		t.Fatalf("expected a vanished run with a resume hint, got:\n%s", stdout)
	}
}

func TestIngestStatusOfAFinishedRunInEnglish(t *testing.T) {
	world := englishWorld()
	world.run("ingest", "git")
	_, stdout, _ := world.run("ingest", "status")
	if !strings.Contains(stdout, "Last ingestion: finished at 2026-09-26 12:00:00, after 0:00") || strings.Contains(stdout, "resume") {
		t.Fatalf("expected a finished run without hints, got:\n%s", stdout)
	}
	requireEnglish(t, stdout)
}

func TestIngestStopTerminatesTheLiveRun(t *testing.T) {
	world := newFakeWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	world.processes.AlivePIDs = []int{77}
	code, stdout, _ := world.run("ingest", "stop")
	if code != 0 || !slices.Equal(world.processes.Terminated, []int{77}) || !strings.Contains(stdout, "cade ingest resume") {
		t.Fatalf("expected pid 77 stopped with a resume hint, got %d %v %q", code, world.processes.Terminated, stdout)
	}
}

func TestIngestStopWithNothingRunning(t *testing.T) {
	world := newFakeWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	code, stdout, _ := world.run("ingest", "stop")
	if code != 0 || len(world.processes.Terminated) != 0 || stdout != "Nenhuma ingestão rodando.\n" {
		t.Fatalf("expected nothing stopped for a gone pid, got %d %v %q", code, world.processes.Terminated, stdout)
	}
}

func TestIngestResumeRunsTheInterruptedArgsAgain(t *testing.T) {
	world := newFakeWorld()
	world.runState.State = ingestrun.State{PID: 77, Args: []string{"git"}, Mode: ingestrun.Mode{Gentle: true}, Status: ingestrun.StatusInterrupted}
	world.runState.Found = true
	code, _, stderr := world.run("ingest", "resume")
	state := world.runState.State
	if code != 0 || world.embedderLoads != 1 || state.Status != ingestrun.StatusFinished || !state.Mode.Gentle || !strings.Contains(stderr, "Retomando: cade ingest git") {
		t.Fatalf("expected the gentle `git` run redone here, got %d %+v %q", code, state, stderr)
	}
}

func TestIngestResumeRestartsABackgroundRunDetached(t *testing.T) {
	world := newFakeWorld()
	world.runState.State = ingestrun.State{PID: 77, Args: []string{"git"}, Mode: ingestrun.Mode{Background: true, Gentle: true}, Status: ingestrun.StatusFailed}
	world.runState.Found = true
	code, _, _ := world.run("ingest", "resume")
	if code != 0 || len(world.processes.Started) != 1 || world.embedderLoads != 0 {
		t.Fatalf("expected a detached restart, got %d with %d started", code, len(world.processes.Started))
	}
}

func TestIngestResumeRefusesALiveOrFinishedRun(t *testing.T) {
	cases := map[string]ingestrun.Status{"ainda está rodando": ingestrun.StatusRunning, "nenhuma ingestão pausada ou interrompida": ingestrun.StatusFinished}
	for want, status := range cases {
		world := newFakeWorld()
		world.runState.State, world.runState.Found = ingestrun.State{PID: 77, Args: []string{"git"}, Status: status}, true
		world.processes.AlivePIDs = []int{77}
		if code, _, stderr := world.run("ingest", "resume"); code != 1 || !strings.Contains(stderr, want) || world.embedderLoads != 0 {
			t.Fatalf("expected %q for a %s run, got %d %q", want, status, code, stderr)
		}
	}
}

func TestIngestRunMarksCancellationAsInterruption(t *testing.T) {
	world := newFakeWorld()
	env := commandEnv{toolkit: world.toolkit()}
	run := newIngestRun(env, []string{"git"}, ingestrun.Mode{})
	run.finish(context.Canceled)
	if state := world.runState.State; state.Status != ingestrun.StatusInterrupted || state.Error != "" {
		t.Fatalf("expected an interruption without an error, got %+v", state)
	}
}

func TestIngestRunRecordsProgressAtMostEverySecond(t *testing.T) {
	world := newFakeWorld()
	clock := &FakeClock{Current: cliNow}
	toolkit := world.toolkit()
	toolkit.Now = clock.Now
	run := newIngestRun(commandEnv{toolkit: toolkit}, []string{"git"}, ingestrun.Mode{})
	run.enterJob(1, 1, "git /repo")
	line := func() string { return "progress" }
	run.update(ingestrun.StageIngesting, line)
	clock.Current = cliNow.Add(recordInterval)
	run.update(ingestrun.StageIngesting, line)
	run.update(ingestrun.StageLoadingEmbedder, line)
	if len(world.runState.Writes) != 3 {
		t.Fatalf("expected the job, the tick after a second and the stage change recorded, got %d writes", len(world.runState.Writes))
	}
}

// FakeEstimatingCollector emits Events and estimates Estimate of them.
type FakeEstimatingCollector struct {
	FakeCollector
	Estimate int
}

func (f FakeEstimatingCollector) EstimateEvents(context.Context) (int, error) { return f.Estimate, nil }

func TestEstimateEventsOnlyFromCollectorsThatCanTell(t *testing.T) {
	env := commandEnv{logger: slog.New(slog.DiscardHandler)}
	estimating := FakeEstimatingCollector{FakeCollector: FakeCollector{Events: []event.Event{sampleCommit}}, Estimate: 40}
	if got := estimateEvents(context.Background(), env, estimating); got != 40 {
		t.Fatalf("expected the collector's 40, got %d", got)
	}
	if got := estimateEvents(context.Background(), env, FakeCollector{}); got != 0 {
		t.Fatalf("expected 0 from a collector without estimates, got %d", got)
	}
}

func TestProgressWithAnEstimateShowsShareAndETA(t *testing.T) {
	clock := &FakeClock{Current: cliNow}
	env := commandEnv{toolkit: Toolkit{Now: clock.Now}}
	progress := newIngestProgress(env, "file /notes", 10)
	for collected := 1; collected <= 2; collected++ {
		clock.Current = clock.Current.Add(time.Second)
		progress.tracker.advance()
	}
	line := progress.line(ingest.Report{Collected: 2}, clock.Current)
	if !strings.Contains(line, "2 lidos de 10 (20%)") || !strings.HasSuffix(line, " · ETA ~8s") {
		t.Fatalf("expected the share and an 8s ETA, got %q", line)
	}
}

func TestFormatElapsedShowsHours(t *testing.T) {
	if got := formatElapsed(time.Hour + 2*time.Minute + 3*time.Second); got != "1:02:03" {
		t.Fatalf("expected 1:02:03, got %q", got)
	}
}
