package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
)

// runDoctor checks the models, SQLite, the database and the configured
// paths, and says what to fix: `cade doctor`. It reads the database
// without migrating it, and fails when a command would.
func runDoctor(ctx context.Context, env commandEnv, args []string) error {
	language := env.toolkit.Language
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
	return doctorVerdict(env.stdout, findings, language)
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

func doctorVerdict(out io.Writer, findings []doctor.Finding, language Language) error {
	failures := doctor.CountBySeverity(findings, doctor.SeverityFailure)
	if failures > 0 {
		return fmt.Errorf(language.pick("%d problema(s) a corrigir, acima", "%d problem(s) to fix, above"), failures)
	}
	warnings := doctor.CountBySeverity(findings, doctor.SeverityWarning)
	fmt.Fprintf(out, language.pick("\nTudo pronto (%d aviso(s)).\n", "\nAll set (%d warning(s)).\n"), warnings)
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
