package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/timeline"
)

var statusLabels = map[tasks.Status]string{tasks.Done: "concluída", tasks.InProgress: "em andamento"}

// renderTaskReport shows the user's tasks, then the ones only consulted;
// tasks that only appeared in other people's messages are summarized
// unless showAll is set.
func renderTaskReport(out io.Writer, days timeline.DayRange, report tasks.Report, showAll bool) {
	groups := groupByInvolvement(report.Tasks)
	if len(groups[tasks.Mine])+len(groups[tasks.Consulted]) == 0 && !showAll {
		fmt.Fprintf(out, "Nenhuma tarefa sua encontrada em %s.\n", days)
		renderOthersSummary(out, groups[tasks.MentionedByOthers])
		return
	}
	fmt.Fprintf(out, "Tarefas de %s — %d suas\n\n", days, len(groups[tasks.Mine]))
	renderTasks(out, groups[tasks.Mine], days)
	renderSection(out, "Consultadas (você abriu a tarefa; sem PR ou mensagem sua)", groups[tasks.Consulted], days)
	if showAll {
		renderSection(out, "Citadas por outras pessoas", groups[tasks.MentionedByOthers], days)
	} else {
		renderOthersSummary(out, groups[tasks.MentionedByOthers])
	}
	if report.UnassignedEvents > 0 {
		fmt.Fprintf(out, "Sem tarefa: %d eventos\n", report.UnassignedEvents)
	}
}

func groupByInvolvement(all []tasks.Task) map[tasks.Involvement][]tasks.Task {
	groups := map[tasks.Involvement][]tasks.Task{}
	for _, task := range all {
		groups[task.Involvement] = append(groups[task.Involvement], task)
	}
	return groups
}

func renderSection(out io.Writer, title string, section []tasks.Task, days timeline.DayRange) {
	if len(section) == 0 {
		return
	}
	fmt.Fprintf(out, "%s:\n\n", title)
	renderTasks(out, section, days)
}

func renderOthersSummary(out io.Writer, others []tasks.Task) {
	if len(others) > 0 {
		fmt.Fprintf(out, "Citadas só por outras pessoas: %d tarefas — use --all para listar.\n", len(others))
	}
}

func renderTasks(out io.Writer, list []tasks.Task, days timeline.DayRange) {
	for _, task := range list {
		renderTask(out, task, days)
	}
}

func renderTask(out io.Writer, task tasks.Task, days timeline.DayRange) {
	fmt.Fprintf(out, "%-13s %s  %s  (%d eventos)\n", statusLabels[task.Status], task.Key, clipLine(task.Title), len(task.Events))
	for _, pr := range task.PRs {
		fmt.Fprintf(out, "%14sPR %s aberto %s%s%s\n", "", pr.Ref.Key(), formatOpenedAt(pr.OpenedAt, days), prTitleSuffix(pr.Title), linkNote(pr.Link))
	}
	fmt.Fprintln(out)
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

func linkNote(link tasks.Link) string {
	if link == tasks.LinkProbable {
		return " (provável)"
	}
	return ""
}
