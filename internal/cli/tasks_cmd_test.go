package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/timeline"
)

func browserVisit(when time.Time, url, title string) event.Event {
	return event.Event{UID: url + when.String(), Source: event.SourceBrowser, Timestamp: when, Content: title,
		Metadata: event.Metadata{"url": url, "title": title}}
}

func TestTasksReportsFinishedTask(t *testing.T) {
	world := newFakeWorld()
	yesterday := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	world.store.Events = []event.Event{
		browserVisit(yesterday, "https://app.proj4.me/projects/14/tasks/162", "Ajuste de CEP"),
		browserVisit(yesterday.Add(20*time.Minute), "https://github.com/acme/api/compare/main...fix-162", "Comparing"),
		browserVisit(yesterday.Add(21*time.Minute), "https://github.com/acme/api/pull/45", "fix-cep-162 by bruno · Pull Request #45 · acme/api · GitHub"),
	}
	code, stdout, _ := world.run("tasks", "ontem")
	if code != 0 || !strings.Contains(stdout, "concluída     14/162  Ajuste de CEP") || !strings.Contains(stdout, "PR acme/api#45 aberto 09:21 · fix-cep-162") {
		t.Fatalf("expected the finished task with its PR, got %d:\n%s", code, stdout)
	}
}

func TestTasksWithoutTasksSaysSo(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("tasks", "2026-01-01")
	if code != 0 || !strings.Contains(stdout, "Nenhuma tarefa sua encontrada em 2026-01-01") {
		t.Fatalf("expected the empty message, got %d %q", code, stdout)
	}
}

func TestTasksRejectsInvalidPattern(t *testing.T) {
	world := newFakeWorld()
	world.cfg.Tasks.TaskURLPatterns = []string{"(unclosed"}
	code, _, stderr := world.run("tasks")
	if code != 1 || !strings.Contains(stderr, `"(unclosed"`) {
		t.Fatalf("expected an error naming the pattern, got %d %q", code, stderr)
	}
}

func TestParseTasksDaysDefaultsToToday(t *testing.T) {
	days, err := parseTasksDays(nil, cliNow)
	if err != nil || days.String() != "2026-09-26" {
		t.Fatalf("expected today, got %v (err %v)", days, err)
	}
}

func TestFormatOpenedAtShowsDateBeforePeriod(t *testing.T) {
	days, _ := timeline.ParseDayRange("2026-09-25", "", cliNow)
	before := formatOpenedAt(time.Date(2026, 9, 12, 16, 40, 0, 0, time.UTC), days)
	within := formatOpenedAt(time.Date(2026, 9, 25, 16, 40, 0, 0, time.UTC), days)
	if before != "12/09 16:40" || within != "16:40" {
		t.Fatalf("unexpected %q / %q", before, within)
	}
}

func TestLinkNoteAndTitleSuffix(t *testing.T) {
	if linkNote(tasks.LinkProbable) != " (provável)" || linkNote(tasks.LinkExact) != "" || prTitleSuffix("") != "" {
		t.Fatal("unexpected link note or title suffix")
	}
}

func teamsLink(when time.Time, text string, sentByMe bool) event.Event {
	mine := "false"
	if sentByMe {
		mine = "true"
	}
	return event.Event{UID: text + when.String(), Source: event.SourceTeams, Timestamp: when, Content: text, Metadata: event.Metadata{"sent_by_me": mine}}
}

// Regression: tasks other developers posted in group chats were listed as
// the user's.
func TestTasksHidesTasksOnlyMentionedByOthers(t *testing.T) {
	world := newFakeWorld()
	yesterday := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	world.store.Events = []event.Event{
		teamsLink(yesterday, "vou pegar https://app.proj4.me/projects/14/tasks/162", true),
		teamsLink(yesterday.Add(time.Hour), "@Ana segue https://app.proj4.me/projects/115/tasks/1420", false),
		browserVisit(yesterday.Add(2*time.Hour), "https://app.proj4.me/projects/227/tasks/36", "Proj4me"),
	}
	_, stdout, _ := world.run("tasks", "ontem")
	if !strings.Contains(stdout, "1 suas") || !strings.Contains(stdout, "14/162") || strings.Contains(stdout, "115/1420") {
		t.Fatalf("expected only the user's task listed, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Consultadas") || !strings.Contains(stdout, "227/36") || !strings.Contains(stdout, "Citadas só por outras pessoas: 1 tarefas") {
		t.Fatalf("expected the consulted task and the others summary, got:\n%s", stdout)
	}
	_, all, _ := world.run("tasks", "--all", "ontem")
	if !strings.Contains(all, "115/1420") {
		t.Fatalf("expected --all to list the others' task, got:\n%s", all)
	}
}

func finishedAndOpenTasks(world *fakeWorld) {
	yesterday := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	world.store.Events = []event.Event{
		browserVisit(yesterday, "https://app.proj4.me/projects/14/tasks/162", "Ajuste de CEP"),
		browserVisit(yesterday.Add(20*time.Minute), "https://github.com/acme/api/compare/main...fix-162", "Comparing"),
		browserVisit(yesterday.Add(21*time.Minute), "https://github.com/acme/api/pull/45", "fix-cep-162 by bruno · Pull Request #45 · acme/api · GitHub"),
		teamsLink(yesterday.Add(time.Hour), "vou pegar https://app.proj4.me/projects/14/tasks/170", true),
	}
}

func TestAskTasksQuestionUsesTaskReport(t *testing.T) {
	world := newFakeWorld()
	finishedAndOpenTasks(world)
	world.generator.StructuredReply = `{"tipo": "tarefas", "periodo": "ontem", "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": "concluidas"}`
	code, stdout, stderr := world.run("ask", "quais tarefas eu finalizei ontem?")
	if code != 0 || world.embedderLoads != 0 || !strings.Contains(stderr, "Entendi: tarefas · 2026-09-25 · concluídas") {
		t.Fatalf("expected a task report without the embedder, got %d, %d loads, %q", code, world.embedderLoads, stderr)
	}
	if !strings.Contains(stdout, "14/162") || strings.Contains(stdout, "14/170") {
		t.Fatalf("expected only the finished task, got:\n%s", stdout)
	}
}

func TestAskTasksWithoutPeriodDefaultsToToday(t *testing.T) {
	world := newFakeWorld()
	world.generator.StructuredReply = `{"tipo": "tarefas", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": null}`
	_, stdout, stderr := world.run("ask", "quais são minhas tarefas?")
	if !strings.Contains(stderr, "Entendi: tarefas · 2026-09-26") || !strings.Contains(stdout, "Nenhuma tarefa sua encontrada em 2026-09-26") {
		t.Fatalf("expected today's report, got %q / %q", stderr, stdout)
	}
}

func TestWithTaskStatus(t *testing.T) {
	list := []tasks.Task{{Key: "a", Status: tasks.Done}, {Key: "b", Status: tasks.InProgress}}
	done, open, all := withTaskStatus(list, queryplan.OnlyDone), withTaskStatus(list, queryplan.OnlyInProgress), withTaskStatus(list, queryplan.AnyStatus)
	if len(done) != 1 || done[0].Key != "a" || len(open) != 1 || open[0].Key != "b" || len(all) != 2 {
		t.Fatalf("unexpected filtering %v / %v / %v", done, open, all)
	}
}
