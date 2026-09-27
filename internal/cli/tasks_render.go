package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/timeline"
)

// statusLabel is a task's status in the report's first column.
func statusLabel(status tasks.Status, language Language) string {
	if status == tasks.Done {
		return language.pick("concluída", "finished")
	}
	return language.pick("em andamento", "in progress")
}

// renderTaskReport shows the user's tasks, then the ones only consulted;
// tasks that only appeared in other people's messages are summarized
// unless showAll is set.
func renderTaskReport(out io.Writer, days timeline.DayRange, report tasks.Report, showAll bool, language Language) {
	groups := groupByInvolvement(report.Tasks)
	if len(groups[tasks.Mine])+len(groups[tasks.Consulted]) == 0 && !showAll {
		fmt.Fprintf(out, language.pick("Nenhuma tarefa sua encontrada em %s.\n", "No tasks of yours found on %s.\n"), days)
		renderOthersSummary(out, groups[tasks.MentionedByOthers], language)
		return
	}
	fmt.Fprintf(out, language.pick("Tarefas de %s — %d suas\n\n", "Tasks of %s — %d yours\n\n"), days, len(groups[tasks.Mine]))
	renderTasks(out, groups[tasks.Mine], days, language)
	renderSection(out, language.pick("Consultadas (você abriu a tarefa; sem PR ou mensagem sua)", "Consulted (you opened the task; no PR or message of yours)"),
		groups[tasks.Consulted], days, language)
	renderOthers(out, groups[tasks.MentionedByOthers], days, showAll, language)
	if report.UnassignedEvents > 0 {
		fmt.Fprintf(out, language.pick("Sem tarefa: %d eventos\n", "Without a task: %d events\n"), report.UnassignedEvents)
	}
}

func renderOthers(out io.Writer, others []tasks.Task, days timeline.DayRange, showAll bool, language Language) {
	if showAll {
		renderSection(out, language.pick("Citadas por outras pessoas", "Mentioned by other people"), others, days, language)
		return
	}
	renderOthersSummary(out, others, language)
}

func groupByInvolvement(all []tasks.Task) map[tasks.Involvement][]tasks.Task {
	groups := map[tasks.Involvement][]tasks.Task{}
	for _, task := range all {
		groups[task.Involvement] = append(groups[task.Involvement], task)
	}
	return groups
}

func renderSection(out io.Writer, title string, section []tasks.Task, days timeline.DayRange, language Language) {
	if len(section) == 0 {
		return
	}
	fmt.Fprintf(out, "%s:\n\n", title)
	renderTasks(out, section, days, language)
}

func renderOthersSummary(out io.Writer, others []tasks.Task, language Language) {
	if len(others) > 0 {
		fmt.Fprintf(out, language.pick("Citadas só por outras pessoas: %d tarefas — use --all para listar.\n",
			"Mentioned only by other people: %d tasks — use --all to list them.\n"), len(others))
	}
}

func renderTasks(out io.Writer, list []tasks.Task, days timeline.DayRange, language Language) {
	for _, task := range list {
		renderTask(out, task, days, language)
	}
}

func renderTask(out io.Writer, task tasks.Task, days timeline.DayRange, language Language) {
	fmt.Fprintf(out, language.pick("%-13s %s  %s  (%d eventos)\n", "%-13s %s  %s  (%d events)\n"), statusLabel(task.Status, language), task.Key, taskTitle(task, language), len(task.Events))
	for _, pr := range task.PRs {
		fmt.Fprintf(out, language.pick("%14sPR %s aberto %s%s%s\n", "%14sPR %s opened %s%s%s\n"),
			"", pr.Ref.Key(), formatOpenedAt(pr.OpenedAt, days), prTitleSuffix(pr.Title), linkNote(pr.Link, language))
	}
	fmt.Fprintln(out)
}

// taskTitle is the task page's title, when a visit to it was seen.
func taskTitle(task tasks.Task, language Language) string {
	if task.Title == "" {
		return language.pick("(sem título)", "(untitled)")
	}
	return clipLine(task.Title)
}

func formatOpenedAt(openedAt time.Time, days timeline.DayRange) string {
	local := openedAt.In(days.First.Location())
	if local.Before(days.Start()) {
		return local.Format("02/01 15:04")
	}
	return local.Format("15:04")
}

func prTitleSuffix(title string) string {
	if title == "" {
		return ""
	}
	return " · " + clipLine(title)
}

func linkNote(link tasks.Link, language Language) string {
	if link == tasks.LinkProbable {
		return language.pick(" (provável)", " (probable)")
	}
	return ""
}
