package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/procctl"
)

// Subcommands of `cade ingest`; none is a source name.
const (
	ingestStartCommand  = "start"
	ingestStatusCommand = "status"
	ingestStopCommand   = "stop"
	ingestPauseCommand  = "pause"
	ingestResumeCommand = "resume"
)

func ingestSubcommands() map[string]subcommand {
	return map[string]subcommand{
		ingestStartCommand:  runIngestStart,
		ingestStatusCommand: runIngestStatus,
		ingestStopCommand:   runIngestStop,
		ingestPauseCommand:  runIngestPause,
		ingestResumeCommand: runIngestResume,
	}
}

// parseForegroundIngestFlags reads the flags of `cade ingest <fonte|all>`.
func parseForegroundIngestFlags(env commandEnv, args []string) (bool, []string, error) {
	flags := newFlagSet("ingest", env.stderr, env.language)
	gentle := flags.Bool("gentle", false, env.language.pick(
		"usa os limites de ingest.background sem sair do terminal (timers, cron)",
		"uses the ingest.background limits without leaving the terminal (timers, cron)"))
	targets, err := parseCommandFlags(flags, args)
	return *gentle, targets, err
}

// runIngestStart is `cade ingest start <fonte|all> [ALVO...]`: the run
// goes on in the background, with the ingest.background limits.
func runIngestStart(_ context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("ingest "+ingestStartCommand, env.stderr, env.language)
	targets, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	return startBackgroundIngest(env, targets)
}

// requireNoIngestArgs rejects arguments to status, stop, pause and resume.
func requireNoIngestArgs(env commandEnv, command string, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf(env.language.pick("cade ingest %s não recebe argumentos, recebido %q", "cade ingest %s takes no arguments, got %q"), command, args)
}

// acquireIngestLock refuses a second ingestion while one is not concluded
// (running or paused), naming it and what to do about it.
func (env commandEnv) acquireIngestLock() (func(), error) {
	release, err := env.toolkit.IngestRuns.Lock.Acquire()
	if !errors.Is(err, ingestrun.ErrLocked) {
		return release, err
	}
	state, _, _ := env.toolkit.IngestRuns.State.Read()
	if state.Status == ingestrun.StatusPaused {
		return nil, fmt.Errorf(env.language.pick("a ingestão (pid %d) está pausada, não concluída; continue com `cade ingest resume` ou pare com `cade ingest stop`",
			"the ingestion (pid %d) is paused, not concluded; continue it with `cade ingest resume` or stop it with `cade ingest stop`"), state.PID)
	}
	return nil, fmt.Errorf(env.language.pick("outra ingestão está rodando (pid %d); acompanhe com `cade ingest status` ou pare com `cade ingest stop`",
		"another ingestion is running (pid %d); follow it with `cade ingest status` or stop it with `cade ingest stop`"), state.PID)
}

// lowerPriorityIfGentle warns instead of failing: the run still works,
// only less politely.
func (env commandEnv) lowerPriorityIfGentle() {
	if !env.ingestRun.isGentle() {
		return
	}
	if err := env.toolkit.IngestRuns.Processes.LowerPriority(); err != nil {
		fmt.Fprintf(env.stderr, env.language.pick("aviso: prioridade não reduzida: %v\n", "warning: priority not lowered: %v\n"), err)
	}
}

// startBackgroundIngest checks the arguments and the lock here, where the
// user still sees the error, then starts a detached cade to do the work.
func startBackgroundIngest(env commandEnv, args []string) error {
	if _, err := env.planIngest(args); err != nil {
		return err
	}
	release, err := env.acquireIngestLock()
	if err != nil {
		return err
	}
	// The child takes the lock itself; this only checked it was free.
	release()
	tools := env.toolkit.IngestRuns
	pid, err := tools.Processes.StartDetached(procctl.DetachedCommand{Args: detachedIngestArgs(env.configPath, args),
		Env: []string{DetachedIngestVariable + "=1"}, LogPath: tools.LogPath})
	if err != nil {
		return err
	}
	printBackgroundStarted(env, pid, tools.LogPath)
	return nil
}

// detachedIngestArgs pass the config explicitly and end the flags, so a
// target starting with "-" stays a target.
func detachedIngestArgs(configPath string, args []string) []string {
	return append([]string{"--config", configPath, "ingest", "--"}, args...)
}

func printBackgroundStarted(env commandEnv, pid int, logPath string) {
	fmt.Fprintf(env.stdout, env.language.pick("Ingestão rodando em segundo plano (pid %d).\n", "Ingestion running in the background (pid %d).\n"), pid)
	fmt.Fprintln(env.stdout, env.language.pick("  acompanhar: cade ingest status", "  follow:     cade ingest status"))
	fmt.Fprintln(env.stdout, env.language.pick("  pausar:     cade ingest pause", "  pause:      cade ingest pause"))
	fmt.Fprintln(env.stdout, env.language.pick("  parar:      cade ingest stop", "  stop:       cade ingest stop"))
	fmt.Fprintf(env.stdout, "  log:        %s\n", logPath)
}

// liveIngest is the run not concluded yet, if any.
func (env commandEnv) liveIngest() (ingestrun.State, bool, error) {
	state, found, err := env.toolkit.IngestRuns.State.Read()
	if err != nil || !found {
		return state, false, err
	}
	return state, state.IsLive(env.toolkit.IngestRuns.Processes.Alive), nil
}

func printNoIngestRunning(env commandEnv) {
	fmt.Fprintln(env.stdout, env.language.pick("Nenhuma ingestão rodando.", "No ingestion running."))
}

// runIngestStop ends the running or paused ingestion, as Ctrl-C would:
// what it stored so far stays, the memory is freed, and `resume` runs it
// again from there.
func runIngestStop(_ context.Context, env commandEnv, args []string) error {
	if err := requireNoIngestArgs(env, ingestStopCommand, args); err != nil {
		return err
	}
	state, live, err := env.liveIngest()
	if err != nil || !live {
		printNoIngestRunning(env)
		return err
	}
	if err := env.toolkit.IngestRuns.Processes.Terminate(state.PID); err != nil {
		return err
	}
	fmt.Fprintf(env.stdout, env.language.pick("Parada pedida à ingestão (pid %d); o que ela já gravou fica. Para retomar: cade ingest resume\n",
		"Asked the ingestion (pid %d) to stop; what it stored so far stays. To resume: cade ingest resume\n"), state.PID)
	return nil
}

// runIngestPause freezes the running ingestion: no CPU or GPU work until
// `resume`, but its models stay in memory and it stays not concluded.
func runIngestPause(_ context.Context, env commandEnv, args []string) error {
	if err := requireNoIngestArgs(env, ingestPauseCommand, args); err != nil {
		return err
	}
	state, live, err := env.liveIngest()
	if err != nil || !live {
		printNoIngestRunning(env)
		return err
	}
	if state.Status == ingestrun.StatusPaused {
		fmt.Fprintf(env.stdout, env.language.pick("A ingestão (pid %d) já está pausada.\n", "The ingestion (pid %d) is already paused.\n"), state.PID)
		return nil
	}
	if err := env.markIngest(state, ingestrun.StatusPaused, env.toolkit.IngestRuns.Processes.Pause); err != nil {
		return err
	}
	fmt.Fprintf(env.stdout, env.language.pick("Ingestão (pid %d) pausada; os modelos continuam na memória. Continuar: cade ingest resume · liberar a memória: cade ingest stop\n",
		"Ingestion (pid %d) paused; its models stay in memory. Continue: cade ingest resume · free the memory: cade ingest stop\n"), state.PID)
	return nil
}

// markIngest signals the run and records its new status: a frozen
// process cannot record it itself. Once it runs again it records its own
// state, marked running, over this one.
func (env commandEnv) markIngest(state ingestrun.State, status ingestrun.Status, signal func(pid int) error) error {
	if err := signal(state.PID); err != nil {
		return err
	}
	state.Status, state.UpdatedAt = status, env.toolkit.Now()
	return env.toolkit.IngestRuns.State.Write(state)
}

// runIngestResume continues a paused ingestion, or runs the last one that
// stopped before finishing again, in the same mode. Running again is
// resuming: stored events and described images are skipped without
// touching the models (RF1.5).
func runIngestResume(ctx context.Context, env commandEnv, args []string) error {
	if err := requireNoIngestArgs(env, ingestResumeCommand, args); err != nil {
		return err
	}
	state, err := env.resumableIngest()
	if err != nil {
		return err
	}
	if state.IsPaused(env.toolkit.IngestRuns.Processes.Alive) {
		return continuePausedIngest(env, state)
	}
	fmt.Fprintf(env.stderr, env.language.pick("Retomando: cade ingest %s\n", "Resuming: cade ingest %s\n"), strings.Join(state.Args, " "))
	if state.Mode.Background {
		return startBackgroundIngest(env, state.Args)
	}
	return runRecordedIngest(ctx, env, state.Args, state.Mode)
}

func continuePausedIngest(env commandEnv, state ingestrun.State) error {
	if err := env.markIngest(state, ingestrun.StatusRunning, env.toolkit.IngestRuns.Processes.Continue); err != nil {
		return err
	}
	fmt.Fprintf(env.stdout, env.language.pick("Ingestão (pid %d) continuando.\n", "Ingestion (pid %d) continuing.\n"), state.PID)
	return nil
}

func (env commandEnv) resumableIngest() (ingestrun.State, error) {
	state, found, err := env.toolkit.IngestRuns.State.Read()
	if err != nil {
		return state, err
	}
	alive := env.toolkit.IngestRuns.Processes.Alive
	if found && state.IsResumable(alive) {
		return state, nil
	}
	if found && state.IsLive(alive) {
		return state, fmt.Errorf(env.language.pick("a ingestão (pid %d) ainda está rodando; acompanhe com `cade ingest status`",
			"the ingestion (pid %d) is still running; follow it with `cade ingest status`"), state.PID)
	}
	return state, errors.New(env.language.pick("nenhuma ingestão pausada ou interrompida para retomar; rode `cade ingest <fonte|all>`",
		"no paused or interrupted ingestion to resume; run `cade ingest <source|all>`"))
}
