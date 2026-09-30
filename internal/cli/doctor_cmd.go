package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
	"github.com/chipskein/cade/internal/ingestrun"
)

// runDoctor checks the models, SQLite, the database and the configured
// paths, and says what to fix: `cade doctor`. It reads the database
// without migrating it, and fails when a command would.
func runDoctor(ctx context.Context, env commandEnv, args []string) error {
	language := env.language
	if len(args) != 0 {
		return fmt.Errorf(language.pick("cade doctor não recebe argumentos, recebido %q", "cade doctor takes no arguments, got %q"), args)
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	database, err := env.toolkit.InspectDatabase(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	findings := doctor.Diagnose(env.toolkit.RootFS, env.configPath, cfg, database)
	home, _ := env.toolkit.HomeDir()
	renderFindings(env.stdout, findings, language, home)
	unfinished, err := env.renderLastIngest()
	if err != nil {
		return err
	}
	return doctorVerdict(env.stdout, findings, language, unfinished)
}

// dayLength turns an age into days for the last ingestion's line.
const dayLength = 24 * time.Hour

var dayNoun = nounForms{"dia", "dias", "day", "days"}

// renderLastIngest prints when the last ingestion ran and how it ended, so
// a forgotten schedule shows before Chrome's ~90 days or the Teams cache
// lose what was never ingested. An unfinished one is a warning, reported
// as its count.
func (env commandEnv) renderLastIngest() (int, error) {
	state, found, err := env.toolkit.IngestRuns.State.Read()
	if err != nil {
		return 0, err
	}
	label := env.language.pick("última ingestão", "last ingestion")
	if !found {
		fmt.Fprintf(env.stdout, "%-6s %-22s %s\n", "—", label, env.language.pick("nenhuma registrada", "none recorded"))
		return 0, nil
	}
	live := state.IsLive(env.toolkit.IngestRuns.Processes.Alive)
	unfinished := state.Status != ingestrun.StatusFinished && !live
	severity := doctor.SeverityOK
	if unfinished {
		severity = doctor.SeverityWarning
	}
	fmt.Fprintf(env.stdout, "%-6s %-22s %s\n", severityLabel(severity, env.language), label, env.lastIngestSummary(state, live))
	if unfinished {
		fmt.Fprintf(env.stdout, "%-29s → %s\n", "", env.language.pick("não terminou; continue com `cade ingest resume`", "did not finish; continue it with `cade ingest resume`"))
	}
	return boolToCount(unfinished), nil
}

// lastIngestSummary is "2026-09-30 05:39:22 (há 2 dias), concluída: cade ingest all".
// A run marked running or paused whose process is gone says so.
func (env commandEnv) lastIngestSummary(state ingestrun.State, live bool) string {
	when := state.FinishedAt
	if when.IsZero() {
		when = state.UpdatedAt
	}
	status := ingestStatusReport{state: state, language: env.language}.statusLabel()
	vanished := !live && (state.Status == ingestrun.StatusRunning || state.Status == ingestrun.StatusPaused)
	if vanished {
		status = env.language.pick("parou sem registrar o fim", "stopped without recording its end")
	}
	return fmt.Sprintf(env.language.pick("%s (há %s), %s: cade ingest %s", "%s (%s ago), %s: cade ingest %s"), when.Format(statusTimeLayout),
		env.ingestAge(env.toolkit.Now().Sub(when)), status, strings.Join(state.Args, " "))
}

// ingestAge is "0:12:03" within a day, "3 dias" after.
func (env commandEnv) ingestAge(age time.Duration) string {
	if age < dayLength {
		return formatElapsed(age)
	}
	return env.language.count(int(age/dayLength), dayNoun)
}

func boolToCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

// renderFindings prints one line per checked item and, under a problem,
// what it means and how to fix it.
func renderFindings(out io.Writer, findings []doctor.Finding, language Language, home string) {
	for _, finding := range findings {
		fmt.Fprintf(out, "%-6s %-22s %s\n", severityLabel(finding.Severity(), language), subjectLabel(finding, language), findingTarget(finding, home))
		if finding.Problem != doctor.ProblemNone {
			fmt.Fprintf(out, "%-29s → %s\n", "", problemText(finding, language))
		}
	}
}

// findingTarget is the checked path, or what was checked when no path.
func findingTarget(finding doctor.Finding, home string) string {
	if finding.Subject == doctor.SubjectKeywordSearch {
		return "SQLite FTS5"
	}
	return config.ContractHome(finding.Path, home)
}

var (
	problemNoun = nounForms{"problema a corrigir", "problemas a corrigir", "problem to fix", "problems to fix"}
	warningNoun = nounForms{"aviso", "avisos", "warning", "warnings"}
)

// doctorVerdict counts extraWarnings (the last ingestion's) with the
// findings' warnings.
func doctorVerdict(out io.Writer, findings []doctor.Finding, language Language, extraWarnings int) error {
	failures := doctor.CountBySeverity(findings, doctor.SeverityFailure)
	if failures > 0 {
		return fmt.Errorf(language.pick("%s, acima", "%s, above"), language.count(failures, problemNoun))
	}
	warnings := doctor.CountBySeverity(findings, doctor.SeverityWarning) + extraWarnings
	fmt.Fprintf(out, language.pick("\nTudo pronto (%s).\n", "\nAll set (%s).\n"), language.count(warnings, warningNoun))
	return nil
}

func severityLabel(severity doctor.Severity, language Language) string {
	switch severity {
	case doctor.SeverityWarning:
		return language.pick("aviso", "warn")
	case doctor.SeverityFailure:
		return language.pick("falha", "fail")
	}
	return "ok"
}

func subjectLabel(finding doctor.Finding, language Language) string {
	labels := map[doctor.Subject]string{
		doctor.SubjectConfigFile:      language.pick("configuração", "config file"),
		doctor.SubjectEmbeddingModel:  language.pick("modelo de embedding", "embedding model"),
		doctor.SubjectGenerationModel: language.pick("modelo de geração", "generation model"),
		doctor.SubjectVisionProjector: language.pick("projetor de visão", "vision projector"),
		doctor.SubjectKeywordSearch:   language.pick("busca por palavras", "keyword search"),
		doctor.SubjectDatabase:        language.pick("banco", "database"),
	}
	if finding.Subject != doctor.SubjectSource {
		return labels[finding.Subject]
	}
	if finding.Source == "" {
		return language.pick("fontes", "sources")
	}
	return language.pick("fonte ", "source ") + finding.Source
}
