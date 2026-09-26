package tasks

import (
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

var day = time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

func at(hour, minute int) time.Time {
	return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

func visit(when time.Time, url, title string) event.Event {
	return event.Event{Source: event.SourceBrowser, Timestamp: when, Content: title, Metadata: event.Metadata{"url": url, "title": title}}
}

func message(when time.Time, text string, sentByMe bool) event.Event {
	mine := "false"
	if sentByMe {
		mine = "true"
	}
	return event.Event{Source: event.SourceTeams, Timestamp: when, Content: text, Metadata: event.Metadata{"sent_by_me": mine}}
}

func commit(when time.Time, message string) event.Event {
	return event.Event{Source: event.SourceGit, Timestamp: when, Content: message}
}

const (
	taskURL    = "https://app.proj4.me/projects/14/tasks/162"
	otherTask  = "https://app.proj4.me/projects/14/tasks/170"
	compareURL = "https://github.com/acme/api/compare/main...fix-cep-162"
	prURL      = "https://github.com/acme/api/pull/45"
)

func build(events []event.Event) Report {
	return NewBuilder(testPatterns).Build(events, events, day.AddDate(0, 0, 1))
}

func taskByKey(report Report, key string) *Task {
	for i := range report.Tasks {
		if report.Tasks[i].Key == key {
			return &report.Tasks[i]
		}
	}
	return nil
}

func TestBuildMarksTaskDoneWhenPRTitleCitesIt(t *testing.T) {
	report := build([]event.Event{
		visit(at(9, 0), taskURL, "Ajuste de CEP"),
		commit(at(9, 10), "corrige CEP"),
		visit(at(9, 25), compareURL, "Comparing changes"),
		visit(at(9, 26), prURL, "fix-cep-162 by bruno · Pull Request #45 · acme/api · GitHub"),
	})
	task := taskByKey(report, "14/162")
	if task == nil || task.Status != Done || len(task.PRs) != 1 || task.PRs[0].Link != LinkExact || task.Title != "Ajuste de CEP" {
		t.Fatalf("expected a done task with an exact PR, got %+v", task)
	}
	if !task.PRs[0].OpenedAt.Equal(at(9, 26)) {
		t.Fatalf("expected the PR opened at 09:26, got %s", task.PRs[0].OpenedAt)
	}
}

func TestBuildLinksPRAnnouncedWithTaskInMessage(t *testing.T) {
	report := build([]event.Event{
		message(at(10, 0), "PR da tarefa "+taskURL+" : "+prURL, true),
	})
	task := taskByKey(report, "14/162")
	if task == nil || task.Status != Done || task.PRs[0].Link != LinkExact {
		t.Fatalf("expected the message to link and finish the task, got %+v", task)
	}
}

func TestBuildProbableLinkWithoutCitation(t *testing.T) {
	report := build([]event.Event{
		visit(at(14, 0), otherTask, "Upload Norteagro"),
		visit(at(14, 30), compareURL, "Comparing changes"),
		visit(at(14, 31), prURL, "upload fix by bruno · Pull Request #45 · acme/api · GitHub"),
	})
	task := taskByKey(report, "14/170")
	if task == nil || task.Status != Done || task.PRs[0].Link != LinkProbable {
		t.Fatalf("expected a probable PR link, got %+v", task)
	}
}

func TestBuildIgnoresPRsOfOthers(t *testing.T) {
	report := build([]event.Event{
		visit(at(9, 0), taskURL, "Ajuste de CEP"),
		visit(at(9, 5), prURL, "fix-cep-162 by ana · Pull Request #45 · acme/api · GitHub"),
	})
	if task := taskByKey(report, "14/162"); task == nil || task.Status != InProgress {
		t.Fatalf("a PR only visited (not opened) must not finish the task, got %+v", task)
	}
}

func TestBuildAmbiguousTitleIsNotExact(t *testing.T) {
	events := []event.Event{
		visit(at(9, 0), "https://app.proj4.me/projects/14/tasks/162", "A"),
		visit(at(9, 1), "https://app.proj4.me/projects/20/tasks/162", "B"),
	}
	linker := newPRLinker(indexPullRequests(events, testPatterns), testPatterns, events)
	if _, ok := uniqueTitleMatch("fix-162", tasksSeen(testPatterns, events)); ok || len(linker.taskOfPR) != 0 {
		t.Fatal("a number shared by two projects must not be linked exactly")
	}
}

func TestBuildAttributesNearbyEventsWithoutLink(t *testing.T) {
	report := build([]event.Event{
		visit(at(9, 0), taskURL, "Ajuste de CEP"),
		commit(at(9, 10), "corrige CEP"), // 10 min after the task page: belongs to it
		commit(at(11, 0), "unrelated"),   // far from any task
	})
	cep := taskByKey(report, "14/162")
	if len(cep.Events) != 2 || report.UnassignedEvents != 1 {
		t.Fatalf("expected the commit attributed and one unassigned event, got %d / %d", len(cep.Events), report.UnassignedEvents)
	}
}

func TestReportListsTasksInOrderFirstTouched(t *testing.T) {
	report := build([]event.Event{
		visit(at(9, 0), otherTask, "Upload"),
		visit(at(9, 5), taskURL, "CEP"),
		visit(at(9, 20), taskURL, "CEP"),
	})
	if report.Tasks[0].Key != "14/170" || report.Tasks[1].Key != "14/162" {
		t.Fatalf("expected chronological order, got %+v", report.Tasks)
	}
}

func TestNearestKeyRespectsSpan(t *testing.T) {
	events := []event.Event{commit(at(9, 0), "a"), commit(at(9, 30), "b")}
	if got := nearestKey(events, []string{"t", ""}, 1); got != "" {
		t.Fatalf("expected no inheritance beyond %s, got %q", inheritSpan, got)
	}
}

func TestMarkOpenedKeepsEarliest(t *testing.T) {
	pr := &PullRequest{}
	pr.markOpened(at(10, 0))
	pr.markOpened(at(9, 0))
	pr.markOpened(at(11, 0))
	if !pr.OpenedByMe || !pr.OpenedAt.Equal(at(9, 0)) {
		t.Fatalf("expected earliest opening 09:00, got %s", pr.OpenedAt)
	}
}

func TestAppendUnique(t *testing.T) {
	if got := appendUnique([]string{"a"}, "a"); len(got) != 1 {
		t.Fatalf("expected no duplicate, got %v", got)
	}
}

func TestAbsDuration(t *testing.T) {
	if absDuration(-time.Minute) != time.Minute || absDuration(time.Minute) != time.Minute {
		t.Fatal("unexpected absolute duration")
	}
}

// Regression: a message listing several tasks linked every PR to the first.
func TestMessageWithSeveralTasksDoesNotLinkExactly(t *testing.T) {
	history := []event.Event{message(at(10, 0), taskURL+" e "+otherTask+": "+prURL, true)}
	if prs := indexPullRequests(history, testPatterns); len(prs["acme/api#45"].TaskKeys) != 0 {
		t.Fatalf("expected no exact link from an ambiguous message, got %v", prs["acme/api#45"].TaskKeys)
	}
}

func TestMessageWithOneTaskLinksEveryPR(t *testing.T) {
	history := []event.Event{message(at(10, 0), taskURL+" PRs: "+prURL+" https://github.com/acme/web/pull/7", true)}
	prs := indexPullRequests(history, testPatterns)
	if prs["acme/api#45"].TaskKeys[0] != "14/162" || prs["acme/web#7"].TaskKeys[0] != "14/162" {
		t.Fatalf("expected both repositories' PRs linked to the task, got %+v", prs)
	}
}

func TestSingleTask(t *testing.T) {
	same := []TaskRef{{Key: "14/162"}, {Key: "14/162"}}
	if key, ok := singleTask(same); !ok || key != "14/162" {
		t.Fatal("repeated links to one task are a single task")
	}
	if _, ok := singleTask([]TaskRef{{Key: "a"}, {Key: "b"}}); ok {
		t.Fatal("two tasks are ambiguous")
	}
}

func TestInvolvement(t *testing.T) {
	report := build([]event.Event{
		message(at(9, 0), "vou fazer "+taskURL, true),
		visit(at(10, 0), otherTask, "Upload"),
		message(at(11, 0), "@Ana segue https://app.proj4.me/projects/115/tasks/1420", false),
	})
	cases := map[string]Involvement{"14/162": Mine, "14/170": Consulted, "115/1420": MentionedByOthers}
	for key, expected := range cases {
		if task := taskByKey(report, key); task == nil || task.Involvement != expected {
			t.Errorf("task %s: expected involvement %d, got %+v", key, expected, task)
		}
	}
}

func TestInvolvementIgnoresInheritedEvents(t *testing.T) {
	// The user's message 5 min later cites nothing: it inherits the task but
	// must not make someone else's task "mine".
	report := build([]event.Event{
		message(at(9, 0), "@Ana segue "+taskURL, false),
		message(at(9, 5), "ok", true),
	})
	if task := taskByKey(report, "14/162"); task.Involvement != MentionedByOthers {
		t.Fatalf("expected MentionedByOthers, got %d", task.Involvement)
	}
}

// Regression: a message citing several tasks only counted for the first,
// so "pega a 162 e a 170" left 170 out of the report.
func TestMessageCitingSeveralTasksCountsForEach(t *testing.T) {
	report := build([]event.Event{message(at(9, 0), "pega a "+taskURL+" e a "+otherTask, false)})
	if len(report.Tasks) != 2 || report.Tasks[0].Key != "14/162" || report.Tasks[1].Key != "14/170" {
		t.Fatalf("expected both tasks, got %+v", report.Tasks)
	}
}

func TestCitedByIgnoresInheritedEvents(t *testing.T) {
	cited := message(at(9, 0), "segue "+taskURL, false)
	nearby := message(at(9, 5), "qualquer coisa", false)
	report := build([]event.Event{cited, nearby, visit(at(11, 0), otherTask, "Upload")})
	builder := NewBuilder(testPatterns)
	if kept := builder.CitedBy(report.Tasks, []event.Event{cited}); len(kept) != 1 || kept[0].Key != "14/162" {
		t.Fatalf("expected only the cited task, got %+v", kept)
	}
	if kept := builder.CitedBy(report.Tasks, []event.Event{nearby}); len(kept) != 0 {
		t.Fatalf("expected an inherited event to cite nothing, got %+v", kept)
	}
}
