package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/timeline"
)

// countedMessage renders one message for a count, so every message with a
// count is checked for 0, 1 and N in both languages.
type countedMessage struct {
	name   string
	render func(n int, language Language) string
	// expected[language] holds the text for 0, 1 and 3.
	expected map[Language][3]string
}

var pluralDay = timeline.DayRange{First: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)}

var countedMessages = []countedMessage{
	{"timeline header", func(n int, l Language) string { return timelineHeader(pluralDay, n, l) }, map[Language][3]string{
		Portuguese: {"Nenhum evento em 2026-09-25.", "Timeline de 2026-09-25 — 1 evento\n", "Timeline de 2026-09-25 — 3 eventos\n"},
		English:    {"No events on 2026-09-25.", "Timeline of 2026-09-25 — 1 event\n", "Timeline of 2026-09-25 — 3 events\n"},
	}},
	{"tasks header", func(n int, l Language) string { return tasksHeader(pluralDay, n, l) }, map[Language][3]string{
		Portuguese: {"— 0 tarefas suas\n", "— 1 tarefa sua\n", "— 3 tarefas suas\n"},
		English:    {"— 0 tasks of yours\n", "— 1 task of yours\n", "— 3 tasks of yours\n"},
	}},
	{"task events", renderTaskWithEvents, map[Language][3]string{
		Portuguese: {"(0 eventos)", "(1 evento)", "(3 eventos)"},
		English:    {"(0 events)", "(1 event)", "(3 events)"},
	}},
	{"tasks without a task", renderUnassigned, map[Language][3]string{
		Portuguese: {"!Sem tarefa", "Sem tarefa: 1 evento\n", "Sem tarefa: 3 eventos\n"},
		English:    {"!Without a task", "Without a task: 1 event\n", "Without a task: 3 events\n"},
	}},
	{"tasks mentioned by others", renderOthersCount, map[Language][3]string{
		Portuguese: {"!Citadas", "Citadas só por outras pessoas: 1 tarefa — use --all para listar.", "3 tarefas — use --all para listar."},
		English:    {"!Mentioned", "Mentioned only by other people: 1 task — use --all to list it.", "3 tasks — use --all to list them."},
	}},
	{"filtered tasks", renderFilteredCount, map[Language][3]string{
		Portuguese: {"Nenhuma tarefa encontrada em 2026-09-25", "Tarefas de 2026-09-25 — 1 tarefa\n", "Tarefas de 2026-09-25 — 3 tarefas\n"},
		English:    {"No tasks found on 2026-09-25", "Tasks of 2026-09-25 — 1 task\n", "Tasks of 2026-09-25 — 3 tasks\n"},
	}},
	{"ingest report", renderIngestCounts, map[Language][3]string{
		Portuguese: {"/repo: 0 novos, 0 atualizados, 0 já existentes (0 lidos)", "/repo: 1 novo, 1 atualizado, 1 já existente, 1 removido da origem (1 lido)",
			"/repo: 3 novos, 3 atualizados, 3 já existentes, 3 removidos da origem (3 lidos)"},
		English: {"/repo: 0 new, 0 updated, 0 already stored (0 read)", "/repo: 1 new, 1 updated, 1 already stored, 1 removed at the source (1 read)",
			"/repo: 3 new, 3 updated, 3 already stored, 3 removed at the source (3 read)"},
	}},
	{"ingest progress", renderIngestProgress, map[Language][3]string{
		Portuguese: {"git: 0 lidos, 0 novos, 0 atualizados, 0 já existentes", "git: 1 lido, 1 novo, 1 atualizado, 1 já existente", "git: 3 lidos, 3 novos"},
		English:    {"git: 0 read, 0 new, 0 updated, 0 already stored", "git: 1 read, 1 new, 1 updated, 1 already stored", "git: 3 read, 3 new"},
	}},
	{"forget", func(n int, l Language) string { return forgottenLine("git", n, l) }, map[Language][3]string{
		Portuguese: {"git: 0 eventos removidos.", "git: 1 evento removido.", "git: 3 eventos removidos."},
		English:    {"git: 0 events removed.", "git: 1 event removed.", "git: 3 events removed."},
	}},
	{"reindex progress", func(n int, l Language) string { return reindexProgressLine(0, n, l) }, map[Language][3]string{
		Portuguese: {"Reindexando: 0/0 eventos", "Reindexando: 0/1 evento (0%)", "Reindexando: 0/3 eventos"},
		English:    {"Reindexing: 0/0 events", "Reindexing: 0/1 event (0%)", "Reindexing: 0/3 events"},
	}},
	{"reindex stopped", func(n int, l Language) string { return reindexStopped(n, errors.New("disk full"), l).Error() }, map[Language][3]string{
		Portuguese: {"parou após 0 eventos (", "parou após 1 evento (", "parou após 3 eventos ("},
		English:    {"stopped after 0 events (", "stopped after 1 event (", "stopped after 3 events ("},
	}},
	{"init sources", renderInitSources, map[Language][3]string{
		Portuguese: {"Fontes: 0 históricos, 0 caches do Teams, 0 repositórios, 0 pastas.", "Fontes: 1 histórico, 1 cache do Teams, 1 repositório, 1 pasta.",
			"Fontes: 3 históricos, 3 caches do Teams, 3 repositórios, 3 pastas."},
		English: {"Sources: 0 histories, 0 Teams caches, 0 repositories, 0 folders.", "Sources: 1 history, 1 Teams cache, 1 repository, 1 folder.",
			"Sources: 3 histories, 3 Teams caches, 3 repositories, 3 folders."},
	}},
	{"doctor warnings", func(n int, l Language) string { return renderDoctorVerdict(n, doctor.ProblemNoDatabase, l) }, map[Language][3]string{
		Portuguese: {"Tudo pronto (0 avisos).", "Tudo pronto (1 aviso).", "Tudo pronto (3 avisos)."},
		English:    {"All set (0 warnings).", "All set (1 warning).", "All set (3 warnings)."},
	}},
	{"doctor problems", func(n int, l Language) string { return renderDoctorVerdict(n, doctor.ProblemMissing, l) }, map[Language][3]string{
		Portuguese: {"Tudo pronto (0 avisos).", "1 problema a corrigir, acima", "3 problemas a corrigir, acima"},
		English:    {"All set (0 warnings).", "1 problem to fix, above", "3 problems to fix, above"},
	}},
	{"teams-schema store", renderStoreCounts, map[Language][3]string{
		Portuguese: {`store "s": 0 registros (0 falhas, 0 em blob)`, `store "s": 1 registro (1 falha, 1 em blob)`, `store "s": 3 registros (3 falhas, 3 em blob)`},
		English:    {`store "s": 0 records (0 failed, 0 in blobs)`, `store "s": 1 record (1 failed, 1 in a blob)`, `store "s": 3 records (3 failed, 3 in blobs)`},
	}},
	{"teams-schema omitted paths", renderOmittedPaths, map[Language][3]string{
		Portuguese: {"!omitid", "(+1 caminho menos frequente omitido)", "(+3 caminhos menos frequentes omitidos)"},
		English:    {"!left out", "(+1 less frequent path left out)", "(+3 less frequent paths left out)"},
	}},
}

// A leading "!" expects the text absent: a zero count hides the line.
func TestCountedMessagesAgreeWithTheCount(t *testing.T) {
	for _, message := range countedMessages {
		for language, expected := range message.expected {
			for i, n := range []int{0, 1, 3} {
				got := message.render(n, language)
				text, absent := strings.CutPrefix(expected[i], "!")
				if strings.Contains(got, text) == absent {
					t.Errorf("%s, language %d, n=%d: expected %q (absent=%v) in %q", message.name, language, n, text, absent, got)
				}
			}
		}
	}
}

func TestCountWritesSingularOnlyForOne(t *testing.T) {
	if Portuguese.count(1, eventNoun) != "1 evento" || Portuguese.count(0, eventNoun) != "0 eventos" || English.count(2, eventNoun) != "2 events" {
		t.Fatal("expected the singular only for exactly one")
	}
}

func renderTaskWithEvents(n int, language Language) string {
	var out strings.Builder
	renderTasks(&out, []tasks.Task{{Key: "14/162", Events: make([]event.Event, n)}}, pluralDay, language)
	return out.String()
}

func renderUnassigned(n int, language Language) string {
	var out strings.Builder
	mine := tasks.Task{Key: "14/162", Involvement: tasks.Mine}
	renderTaskReport(&out, pluralDay, tasks.Report{Tasks: []tasks.Task{mine}, UnassignedEvents: n}, false, language)
	return out.String()
}

func renderOthersCount(n int, language Language) string {
	var out strings.Builder
	renderOthersSummary(&out, make([]tasks.Task, n), language)
	return out.String()
}

func renderFilteredCount(n int, language Language) string {
	var out strings.Builder
	renderFilteredTasks(&out, pluralDay, make([]tasks.Task, n), language)
	return out.String()
}

func renderIngestCounts(n int, language Language) string {
	return ingestReportLine("/repo", ingest.Report{Collected: n, Inserted: n, Updated: n, AlreadyStored: n, Removed: n}, language)
}

func renderIngestProgress(n int, language Language) string {
	progress := ingestProgress{label: "git", language: language, started: cliNow}
	return progress.line(ingest.Report{Collected: n, Inserted: n, Updated: n, AlreadyStored: n}, cliNow.Add(time.Second))
}

func renderInitSources(n int, language Language) string {
	paths := make([]string, n)
	return initSourcesLine(config.SourcesConfig{BrowserHistories: paths, TeamsIndexedDBDirs: paths, GitRepositories: paths, Directories: paths}, language)
}

func renderDoctorVerdict(n int, problem doctor.Problem, language Language) string {
	findings := make([]doctor.Finding, n)
	for i := range findings {
		findings[i] = doctor.Finding{Problem: problem}
	}
	var out strings.Builder
	if err := doctorVerdict(&out, findings, language); err != nil {
		return err.Error()
	}
	return out.String()
}

func renderStoreCounts(n int, language Language) string {
	var out strings.Builder
	renderStoreSummary(&out, idbschema.StoreSummary{Database: "d", Store: "s", Records: n, Failed: n, BlobWrapped: n}, language)
	return out.String()
}

func renderOmittedPaths(n int, language Language) string {
	var out strings.Builder
	renderStoreSummary(&out, idbschema.StoreSummary{Fields: make([]idbschema.FieldStat, maxFieldsPerStore+n)}, language)
	return out.String()
}
