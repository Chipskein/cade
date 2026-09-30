package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/ingestrun"
)

// statusTimeLayout dates the start and end of a run in `ingest status`.
const statusTimeLayout = "2006-01-02 15:04:05"

// runIngestStatus prints the running, paused or last ingestion, as its
// process recorded it, from any terminal.
func runIngestStatus(_ context.Context, env commandEnv, args []string) error {
	if err := requireNoIngestArgs(env, ingestStatusCommand, args); err != nil {
		return err
	}
	state, found, err := env.toolkit.IngestRuns.State.Read()
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(env.stdout, env.language.pick("Nenhuma ingestão registrada ainda.", "No ingestion recorded yet."))
		return nil
	}
	report := ingestStatusReport{state: state, live: state.IsLive(env.toolkit.IngestRuns.Processes.Alive), now: env.toolkit.Now(), language: env.language}
	fmt.Fprint(env.stdout, report.String())
	return nil
}

// ingestStatusReport is the text of `ingest status` for one state.
type ingestStatusReport struct {
	state    ingestrun.State
	live     bool
	now      time.Time
	language Language
}

func (r ingestStatusReport) String() string {
	lines := []string{r.headline()}
	if r.state.Job != "" {
		lines = append(lines, fmt.Sprintf(r.language.pick("  etapa %d de %d: %s", "  job %d of %d: %s"), r.state.JobNumber, r.state.JobCount, r.state.Job))
	}
	if r.state.Progress != "" {
		lines = append(lines, "  "+r.state.Progress)
	}
	if r.state.Error != "" {
		lines = append(lines, r.language.pick("  erro: ", "  error: ")+r.state.Error)
	}
	return strings.Join(append(lines, r.hints()...), "\n") + "\n"
}

func (r ingestStatusReport) headline() string {
	started := r.state.StartedAt.Format(statusTimeLayout)
	if r.live && r.state.Status == ingestrun.StatusPaused {
		return fmt.Sprintf(r.language.pick("Ingestão pausada (pid %d%s), iniciada em %s; pausada há %s",
			"Ingestion paused (pid %d%s), started at %s; paused %s ago"), r.state.PID, r.modeLabel(), started,
			formatElapsed(r.now.Sub(r.state.UpdatedAt)))
	}
	if r.live {
		return fmt.Sprintf(r.language.pick("Ingestão em andamento (pid %d%s), desde %s (%s); atualizada há %s",
			"Ingestion running (pid %d%s), since %s (%s); updated %s ago"), r.state.PID, r.modeLabel(), started,
			formatElapsed(r.now.Sub(r.state.StartedAt)), formatElapsed(r.now.Sub(r.state.UpdatedAt)))
	}
	if r.state.Status == ingestrun.StatusRunning || r.state.Status == ingestrun.StatusPaused {
		return fmt.Sprintf(r.language.pick("Última ingestão: parou sem registrar o fim (pid %d não existe mais); última atualização em %s",
			"Last ingestion: stopped without recording its end (pid %d is gone); last updated at %s"), r.state.PID, r.state.UpdatedAt.Format(statusTimeLayout))
	}
	return fmt.Sprintf(r.language.pick("Última ingestão: %s em %s, depois de %s", "Last ingestion: %s at %s, after %s"), r.statusLabel(),
		r.state.FinishedAt.Format(statusTimeLayout), formatElapsed(r.state.FinishedAt.Sub(r.state.StartedAt)))
}

// modeLabel is ", em segundo plano, modo gentil" or "".
func (r ingestStatusReport) modeLabel() string {
	label := ""
	if r.state.Mode.Background {
		label += r.language.pick(", em segundo plano", ", in the background")
	}
	if r.state.Mode.Gentle {
		label += r.language.pick(", modo gentil", ", gentle")
	}
	return label
}

func (r ingestStatusReport) statusLabel() string {
	switch r.state.Status {
	case ingestrun.StatusFinished:
		return r.language.pick("concluída", "finished")
	case ingestrun.StatusInterrupted:
		return r.language.pick("interrompida", "interrupted")
	case ingestrun.StatusRunning:
		return r.language.pick("em andamento", "running")
	case ingestrun.StatusPaused:
		return r.language.pick("pausada", "paused")
	}
	return r.language.pick("falhou", "failed")
}

// hints say what can be done next: pause or stop a running run, resume
// a paused or stopped one.
func (r ingestStatusReport) hints() []string {
	paused := r.live && r.state.Status == ingestrun.StatusPaused
	var hints []string
	if r.live && !paused {
		hints = append(hints, r.language.pick("  pausar:   cade ingest pause", "  pause:    cade ingest pause"))
	}
	if paused || !r.live && r.state.Status != ingestrun.StatusFinished {
		hints = append(hints, r.language.pick("  retomar:  cade ingest resume", "  resume:   cade ingest resume"))
	}
	if r.live {
		hints = append(hints, r.language.pick("  parar:    cade ingest stop", "  stop:     cade ingest stop"))
	}
	if r.state.LogPath != "" {
		hints = append(hints, "  log:      "+r.state.LogPath)
	}
	return hints
}
