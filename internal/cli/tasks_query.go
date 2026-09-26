package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/textnorm"
	"github.com/chipskein/cade/internal/timeline"
)

// tasksForQuery answers "quais tarefas finalizei ontem?" with the same report
// as `cade tasks`, straight from the database: no model summary, so no
// invented task or status. People, direction and topic narrow it to, e.g.,
// the tasks Ana passed on.
func (env commandEnv) tasksForQuery(ctx context.Context, cfg config.Config, store storage.EventStore, query queryplan.Query, session *askSession) error {
	patterns, err := compileTaskPatterns(cfg.Tasks.TaskURLPatterns)
	if err != nil {
		return err
	}
	report, err := buildTaskReport(ctx, store, patterns, *query.Days)
	if err != nil {
		return err
	}
	report.Tasks = withTaskStatus(report.Tasks, query.TaskStatus)
	session.status.clear()
	if query.Criteria.IsEmpty() && query.Topic == "" {
		return env.showTaskReport(query, report, session)
	}
	matching := tasksMatching(tasks.NewBuilder(patterns), report.Tasks, query, env.stderr)
	if session.jsonOutput {
		return session.writeReport(query, func(json *askReport) { json.Tasks = taskReports(matching) })
	}
	renderFilteredTasks(env.stdout, *query.Days, matching)
	return nil
}

// showTaskReport prints the unfiltered report; the JSON lists every task,
// with its involvement, instead of summarizing others' tasks.
func (env commandEnv) showTaskReport(query queryplan.Query, report tasks.Report, session *askSession) error {
	if session.jsonOutput {
		return session.writeReport(query, func(json *askReport) { json.Tasks = taskReports(report.Tasks) })
	}
	renderTaskReport(env.stdout, *query.Days, report, false)
	return nil
}

var wantedStatus = map[queryplan.TaskStatus]tasks.Status{queryplan.OnlyDone: tasks.Done, queryplan.OnlyInProgress: tasks.InProgress}

// withTaskStatus keeps the tasks in the requested status.
func withTaskStatus(list []tasks.Task, status queryplan.TaskStatus) []tasks.Task {
	wanted, filtered := wantedStatus[status]
	if !filtered {
		return list
	}
	var kept []tasks.Task
	for _, task := range list {
		if task.Status == wanted {
			kept = append(kept, task)
		}
	}
	return kept
}

// tasksMatching keeps the tasks cited in the messages the people and
// direction select ("que a Ana me passou"), then those mentioning the
// topic. A name that matches nobody is searched as a topic instead:
// in "tarefas de Fertalvo" the model read a client as a person.
func tasksMatching(builder tasks.Builder, list []tasks.Task, query queryplan.Query, out io.Writer) []tasks.Task {
	kept, matched, unknown := query.Criteria.Apply(taskEvents(list))
	reportPeople(out, matched, nil)
	reportNamesAsText(out, unknown)
	if len(matched) > 0 || query.Criteria.Direction != listing.AnyDirection {
		list = builder.CitedBy(list, messagesOnly(kept))
	}
	terms := unknown
	if query.Topic != "" {
		terms = append(terms, query.Topic)
	}
	return mentioningAll(list, terms)
}

func taskEvents(list []tasks.Task) []event.Event {
	var all []event.Event
	for _, task := range list {
		all = append(all, task.Events...)
	}
	return all
}

// messagesOnly drops pages and commits: only a message passes a task on,
// and the direction filter lets every non-message event through.
func messagesOnly(events []event.Event) []event.Event {
	var messages []event.Event
	for _, ev := range events {
		if ev.Source == event.SourceTeams {
			messages = append(messages, ev)
		}
	}
	return messages
}

// mentioningAll keeps the tasks whose title, PRs or events contain every
// term, ignoring case and accents ("fertalvo" matches "-> main-fertalvo").
func mentioningAll(list []tasks.Task, terms []string) []tasks.Task {
	var kept []tasks.Task
	for _, task := range list {
		if containsAllTerms(taskText(task), terms) {
			kept = append(kept, task)
		}
	}
	return kept
}

func taskText(task tasks.Task) string {
	parts := []string{task.Key, task.Title}
	for _, pr := range task.PRs {
		parts = append(parts, pr.Ref.Key(), pr.Title)
	}
	for _, ev := range task.Events {
		parts = append(parts, ev.Content)
	}
	return textnorm.Fold(strings.Join(parts, "\n"))
}

// renderFilteredTasks lists the narrowed tasks whoever they belong to: the
// tasks someone passed on are usually not yet the user's.
func renderFilteredTasks(out io.Writer, days timeline.DayRange, list []tasks.Task) {
	if len(list) == 0 {
		fmt.Fprintf(out, "Nenhuma tarefa encontrada em %s com esses filtros (tarefas são reconhecidas por links de tarefa nas mensagens e páginas).\n", days)
		return
	}
	fmt.Fprintf(out, "Tarefas de %s — %d\n\n", days, len(list))
	renderTasks(out, list, days)
}
