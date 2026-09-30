package cli

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/pacing"
)

// recordInterval is how often a run's progress reaches the state file:
// often enough for `ingest status`, rare enough to cost nothing.
const recordInterval = time.Second

// ingestRun records one `ingest` in the state file and applies its mode:
// the background limits and the pacing of the models. Its methods do
// nothing on a nil run, so commands other than ingest pass through.
type ingestRun struct {
	file        ingestrun.StateFile
	state       ingestrun.State
	now         func() time.Time
	clock       pacing.Clock
	logger      *slog.Logger
	written     time.Time
	writeFailed bool
}

func newIngestRun(env commandEnv, args []string, mode ingestrun.Mode) *ingestRun {
	tools := env.toolkit.IngestRuns
	state := ingestrun.State{PID: tools.PID, Args: args, Mode: mode, Status: ingestrun.StatusRunning,
		Stage: ingestrun.StageStarting, StartedAt: env.toolkit.Now()}
	if mode.Background {
		state.LogPath = tools.LogPath
	}
	return &ingestRun{file: tools.State, state: state, now: env.toolkit.Now, clock: tools.Clock, logger: env.logger}
}

func (r *ingestRun) isGentle() bool {
	return r != nil && r.state.Mode.Gentle
}

// start records the run before anything slow happens.
func (r *ingestRun) start() {
	r.write(r.now())
}

// enterJob records which job of the plan runs now.
func (r *ingestRun) enterJob(number, count int, label string) {
	if r == nil {
		return
	}
	r.state.Job, r.state.JobNumber, r.state.JobCount = label, number, count
	r.state.Stage, r.state.Progress = ingestrun.StageIngesting, ""
	r.write(r.now())
}

// update records what the run is doing: at once when the stage changes,
// otherwise at most every recordInterval, building line only then.
func (r *ingestRun) update(stage ingestrun.Stage, line func() string) {
	if r == nil {
		return
	}
	now := r.now()
	if stage == r.state.Stage && now.Sub(r.written) < recordInterval {
		return
	}
	r.state.Stage, r.state.Progress = stage, line()
	r.write(now)
}

// finish records how the run ended: Ctrl-C, `ingest stop` and a closed
// terminal all cancel the context, which is an interruption.
func (r *ingestRun) finish(err error) {
	now := r.now()
	r.state.FinishedAt, r.state.Status = now, ingestrun.StatusFinished
	if errors.Is(err, context.Canceled) {
		r.state.Status = ingestrun.StatusInterrupted
	} else if err != nil {
		r.state.Status, r.state.Error = ingestrun.StatusFailed, err.Error()
	}
	r.write(now)
}

// write never fails the ingestion: the state only serves `ingest status`. It
// warns once, not on every progress tick.
func (r *ingestRun) write(now time.Time) {
	r.state.UpdatedAt, r.written = now, now
	err := r.file.Write(r.state)
	if err == nil || r.writeFailed {
		return
	}
	r.writeFailed = true
	r.logger.Warn("ingest state not recorded; `cade ingest status` will be out of date", "error", err.Error())
}

// paceEmbedder rests the embedder between calls in gentle runs.
func (r *ingestRun) paceEmbedder(ctx context.Context, cfg config.Config, embedder ClosableEmbedder) ClosableEmbedder {
	if !r.isGentle() {
		return embedder
	}
	return pacing.NewPacedEmbedder(ctx, embedder, r.dutyCycle(cfg))
}

// paceDescriber rests the vision model between images in gentle runs.
func (r *ingestRun) paceDescriber(cfg config.Config, describer imagecaption.ClosableDescriber) imagecaption.ClosableDescriber {
	if !r.isGentle() {
		return describer
	}
	return pacing.NewPacedDescriber(describer, r.dutyCycle(cfg))
}

func (r *ingestRun) dutyCycle(cfg config.Config) *pacing.DutyCycle {
	return pacing.NewDutyCycle(cfg.Ingest.Background.BusyPercent, r.clock)
}
