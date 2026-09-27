package cli

import (
	"fmt"

	"github.com/chipskein/cade/internal/doctor"
)

// problemWording says what a problem means and how to fix it, in each
// language; the finding supplies the values it names.
type problemWording func(finding doctor.Finding, language Language) string

var problemWordings = map[doctor.Problem]problemWording{
	doctor.ProblemNoConfigFile: fixed("não existe, valem os padrões; rode `cade init`", "does not exist, defaults apply; run `cade init`"),
	doctor.ProblemNoSources:    fixed("nenhuma fonte configurada; rode `cade init` ou edite `sources`", "no source configured; run `cade init` or edit `sources`"),
	doctor.ProblemNoDatabase:   fixed("ainda não existe; o primeiro `cade ingest` cria", "does not exist yet; the first `cade ingest` creates it"),
	doctor.ProblemMissing:      missingWording,
	doctor.ProblemNotAFile:     withSetting("é um diretório; `%s` espera um arquivo", "is a directory; `%s` expects a file"),
	doctor.ProblemNotADirectory: withSetting("não é um diretório; `%s` espera um diretório",
		"is not a directory; `%s` expects a directory"),
	doctor.ProblemNotGGUF: fixed("não é um modelo GGUF (download incompleto?); rode `make models` de novo",
		"is not a GGUF model (incomplete download?); run `make models` again"),
	doctor.ProblemNotSQLite: fixed("não é um histórico do Chrome (History) nem do Firefox (places.sqlite)",
		"is not a Chrome (History) or Firefox (places.sqlite) history"),
	doctor.ProblemNotGitRepository: fixed("não é um repositório git (não tem .git)", "is not a git repository (no .git)"),
	doctor.ProblemNotLevelDB: fixed("não é um IndexedDB do Chrome (não tem CURRENT)",
		"is not a Chrome IndexedDB (no CURRENT)"),
	doctor.ProblemNoFTS5: fixed("SQLite sem FTS5: este binário não foi compilado com `make`; compile de novo",
		"SQLite without FTS5: this binary was not built with `make`; rebuild it"),
	doctor.ProblemSchemaTooNew:      schemaTooNewWording,
	doctor.ProblemEmbeddingMismatch: embeddingMismatchWording,
	doctor.ProblemReindexPending: fixed("reindexação incompleta: parte dos eventos está sem vetor; rode `cade reindex`",
		"unfinished reindex: some events have no vector; run `cade reindex`"),
	doctor.ProblemMigrationPending: migrationWording,
}

// problemText words the finding's problem.
func problemText(finding doctor.Finding, language Language) string {
	wording, known := problemWordings[finding.Problem]
	if !known {
		return fmt.Sprintf("problem %d", finding.Problem)
	}
	return wording(finding, language)
}

func fixed(portuguese, english string) problemWording {
	return func(_ doctor.Finding, language Language) string { return language.pick(portuguese, english) }
}

func withSetting(portuguese, english string) problemWording {
	return func(finding doctor.Finding, language Language) string {
		return fmt.Sprintf(language.pick(portuguese, english), finding.Setting)
	}
}

func missingWording(finding doctor.Finding, language Language) string {
	if finding.Subject == doctor.SubjectEmbeddingModel || finding.Subject == doctor.SubjectGenerationModel {
		return fmt.Sprintf(language.pick("não existe; rode `make models` ou ajuste `%s`", "does not exist; run `make models` or set `%s`"), finding.Setting)
	}
	return fmt.Sprintf(language.pick("não existe; corrija ou remova de `%s`", "does not exist; fix it or remove it from `%s`"), finding.Setting)
}

func schemaTooNewWording(finding doctor.Finding, language Language) string {
	return fmt.Sprintf(language.pick("esquema v%d, mais novo que este cade (v%d); atualize o cade", "schema v%d, newer than this cade (v%d); update cade"),
		finding.Database.SchemaVersion, finding.Database.LatestSchemaVersion)
}

func embeddingMismatchWording(finding doctor.Finding, language Language) string {
	return fmt.Sprintf(language.pick("vetores de %s, mas a configuração usa %s; rode `cade reindex`", "vectors from %s, but the config uses %s; run `cade reindex`"),
		finding.Database.EmbeddingModel, finding.ConfiguredModel)
}

// migrationWording warns of the copy: a real history needed ~1 minute and
// as much free disk as the database.
func migrationWording(finding doctor.Finding, language Language) string {
	database := finding.Database
	text := fmt.Sprintf(language.pick("esquema v%d; o próximo comando migra para v%d", "schema v%d; the next command migrates it to v%d"),
		database.SchemaVersion, database.LatestSchemaVersion)
	if !database.MigrationBackup {
		return text
	}
	megabytes := (database.SizeBytes + 1<<20 - 1) >> 20
	return text + fmt.Sprintf(language.pick(", antes gravando uma cópia (~%d MB)", ", first writing a copy (~%d MB)"), megabytes)
}
