package tasks

import (
	"regexp"
	"sort"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// probableWindow: a PR opened by the user within this time after working
// on a task, with no explicit link, is probably that task's.
const probableWindow = 2 * time.Hour

// prLinker resolves which tasks events and pull requests belong to.
type prLinker struct {
	prs          map[string]*PullRequest
	taskPatterns []*regexp.Regexp
	// taskOfPR holds exact PR → task links.
	taskOfPR map[string]string
}

// newPRLinker links PRs to tasks exactly: tasks named in a message with the
// PR link, else the task whose identifier the PR title cites — among tasks
// seen in the period, so an identifier shared by two projects is not guessed.
func newPRLinker(prs map[string]*PullRequest, taskPatterns []*regexp.Regexp, periodEvents []event.Event) *prLinker {
	linker := &prLinker{prs: prs, taskPatterns: taskPatterns, taskOfPR: map[string]string{}}
	periodTasks := tasksSeen(taskPatterns, periodEvents)
	for key, pr := range prs {
		if len(pr.TaskKeys) > 0 {
			linker.taskOfPR[key] = pr.TaskKeys[0]
			continue
		}
		if task, ok := uniqueTitleMatch(pr.Title, periodTasks); ok {
			linker.taskOfPR[key] = task
		}
	}
	return linker
}

func tasksSeen(taskPatterns []*regexp.Regexp, events []event.Event) []TaskRef {
	var seen []TaskRef
	keys := map[string]bool{}
	for _, ev := range events {
		for _, ref := range taskRefs(taskPatterns, refText(ev)) {
			if !keys[ref.Key] {
				keys[ref.Key] = true
				seen = append(seen, ref)
			}
		}
	}
	return seen
}

func uniqueTitleMatch(title string, tasks []TaskRef) (string, bool) {
	var matches []string
	for _, task := range tasks {
		if title != "" && citesID(title, task.ID) {
			matches = append(matches, task.Key)
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// directTask is the task an event cites: a task link, or a link to a PR
// already tied to a task.
func (l *prLinker) directTask(ev event.Event) string {
	text := refText(ev)
	if refs := taskRefs(l.taskPatterns, text); len(refs) > 0 {
		return refs[0].Key
	}
	for _, ref := range prRefs(text) {
		if task := l.taskOfPR[ref.Key()]; task != "" {
			return task
		}
	}
	return ""
}

// attachPRs adds to each task the PRs the user opened by periodEnd: exact
// links first, then PRs with no link opened soon after work on the task.
func (l *prLinker) attachPRs(tasks map[string]*Task, periodEnd time.Time) {
	for _, pr := range l.openedPRs(periodEnd) {
		key, link := l.taskOfPR[pr.Ref.Key()], LinkExact
		if key == "" {
			key, link = probableTask(tasks, pr.OpenedAt), LinkProbable
		}
		if task := tasks[key]; task != nil {
			task.PRs = append(task.PRs, LinkedPR{PullRequest: *pr, Link: link})
			task.Status = Done
		}
	}
}

func (l *prLinker) openedPRs(periodEnd time.Time) []*PullRequest {
	var opened []*PullRequest
	for _, pr := range l.prs {
		if pr.OpenedByMe && pr.OpenedAt.Before(periodEnd) {
			opened = append(opened, pr)
		}
	}
	sort.Slice(opened, func(i, j int) bool { return opened[i].OpenedAt.Before(opened[j].OpenedAt) })
	return opened
}

// probableTask is the task with the latest event before openedAt, if that
// event is within probableWindow.
func probableTask(tasks map[string]*Task, openedAt time.Time) string {
	best, bestTime := "", time.Time{}
	for key, task := range tasks {
		for _, ev := range task.Events {
			gap := openedAt.Sub(ev.Timestamp)
			if gap >= 0 && gap <= probableWindow && ev.Timestamp.After(bestTime) {
				best, bestTime = key, ev.Timestamp
			}
		}
	}
	return best
}
