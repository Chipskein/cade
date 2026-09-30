package tasks

import (
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// Status is how far a task got, by the user's rule: a task is finished
// once its pull request is open.
type Status int

const (
	InProgress Status = iota
	Done
)

// Link says how sure the PR ↔ task association is.
type Link int

const (
	// LinkExact: a message carried both links, or the PR title cites the
	// task number.
	LinkExact Link = iota
	// LinkProbable: the PR was opened right after working on the task.
	LinkProbable
)

// LinkedPR is a pull request attached to a task.
type LinkedPR struct {
	PullRequest
	Link Link
}

// Involvement says whether a task is the user's: links to tasks also show
// up in other people's messages about their own work.
type Involvement int

const (
	// Mine: the user opened a PR for it, sent a message or made a commit
	// citing it.
	Mine Involvement = iota
	// Consulted: the user opened the task page, nothing more.
	Consulted
	// MentionedByOthers: it only appeared in other people's messages.
	MentionedByOthers
)

// Task is one task's summary for the period.
type Task struct {
	Key string
	// Title is the task page's title; empty when no visit to it was seen
	// (the CLI words the gap in the user's language).
	Title       string
	Status      Status
	Involvement Involvement
	PRs         []LinkedPR
	// Events are the period's events attributed to the task, in time order.
	Events []event.Event
}

// Report lists the period's tasks in the order they were first touched.
type Report struct {
	Tasks            []Task
	UnassignedEvents int
}

// Builder turns events into a Report.
type Builder struct {
	taskPatterns []*regexp.Regexp
}

// NewBuilder recognizes tasks by taskPatterns: URL regexes whose capture
// groups identify a task, the last being the identifier written in branch
// names and PR titles ("162", "PROJ-123").
//
//	builder := tasks.NewBuilder([]*regexp.Regexp{regexp.MustCompile(`/browse/([A-Z]+-\d+)`)})
func NewBuilder(taskPatterns []*regexp.Regexp) Builder {
	return Builder{taskPatterns: taskPatterns}
}

// Build reports on periodEvents (sorted by time). history is a longer span
// ending with the period, used to find when PRs were opened.
func (b Builder) Build(periodEvents, history []event.Event, periodEnd time.Time) Report {
	prs := indexPullRequests(history, b.taskPatterns)
	linker := newPRLinker(prs, b.taskPatterns, periodEvents)
	keys := assignTasks(periodEvents, linker.directTask)
	var report Report
	byKey := b.collectTasks(periodEvents, keys, &report)
	linker.attachPRs(byKey, periodEnd)
	for _, task := range byKey {
		task.Involvement = b.involvement(task)
	}
	report.Tasks = sortedTasks(byKey)
	return report
}

// involvement classifies by the strongest signal among the events that
// cite the task directly (inherited events say nothing about ownership).
func (b Builder) involvement(task *Task) Involvement {
	level := MentionedByOthers
	for _, pr := range task.PRs {
		if pr.OpenedByMe {
			return Mine
		}
	}
	for _, ev := range task.Events {
		if !b.cites(ev, task.Key) {
			continue
		}
		if ev.Source == event.SourceTeams && ev.Message().SentByMe || ev.Source == event.SourceGit && !ev.IsOthersCommit() {
			return Mine
		}
		if ev.Source == event.SourceBrowser {
			level = Consulted
		}
	}
	return level
}

func (b Builder) cites(ev event.Event, key string) bool {
	for _, ref := range taskRefs(b.taskPatterns, refText(ev)) {
		if ref.Key == key {
			return true
		}
	}
	return false
}

func (b Builder) collectTasks(events []event.Event, keys []string, report *Report) map[string]*Task {
	byKey := map[string]*Task{}
	for i, ev := range events {
		taskKeys := b.taskKeysOf(ev, keys[i])
		if len(taskKeys) == 0 {
			report.UnassignedEvents++
		}
		for _, key := range taskKeys {
			b.addEvent(byKey, key, ev)
		}
	}
	return byKey
}

// taskKeysOf is the event's assigned task plus every other task it cites.
// Regression: a message citing several tasks ("pega a 162 e a 170") only
// counted for the first, so the others were missing from the report.
func (b Builder) taskKeysOf(ev event.Event, assigned string) []string {
	if assigned == "" {
		return nil
	}
	keys := []string{assigned}
	for _, ref := range taskRefs(b.taskPatterns, refText(ev)) {
		if !slices.Contains(keys, ref.Key) {
			keys = append(keys, ref.Key)
		}
	}
	return keys
}

func (b Builder) addEvent(byKey map[string]*Task, key string, ev event.Event) {
	task := byKey[key]
	if task == nil {
		task = &Task{Key: key}
		byKey[key] = task
	}
	task.Events = append(task.Events, ev)
	b.adoptTitle(task, ev)
}

// CitedBy keeps the tasks that one of events cites directly with a task
// link, e.g. the tasks a person passed on in their messages. Events merely
// inherited by a task (close in time) do not count.
//
//	fromAna := builder.CitedBy(report.Tasks, anasMessages)
func (b Builder) CitedBy(tasks []Task, events []event.Event) []Task {
	cited := map[string]bool{}
	for _, ev := range events {
		for _, ref := range taskRefs(b.taskPatterns, refText(ev)) {
			cited[ref.Key] = true
		}
	}
	var kept []Task
	for _, task := range tasks {
		if cited[task.Key] {
			kept = append(kept, task)
		}
	}
	return kept
}

// adoptTitle names the task after its page title in the browser.
func (b Builder) adoptTitle(task *Task, ev event.Event) {
	visit := ev.Visit()
	if ev.Source != event.SourceBrowser || visit.Title == "" {
		return
	}
	for _, ref := range taskRefs(b.taskPatterns, visit.URL) {
		if ref.Key == task.Key {
			task.Title = visit.Title
		}
	}
}

func sortedTasks(byKey map[string]*Task) []Task {
	tasks := make([]Task, 0, len(byKey))
	for _, task := range byKey {
		tasks = append(tasks, *task)
	}
	// Every task has at least one event, and events arrive in time order.
	sort.Slice(tasks, func(i, j int) bool {
		first, other := tasks[i].Events[0].Timestamp, tasks[j].Events[0].Timestamp
		if !first.Equal(other) {
			return first.Before(other)
		}
		return tasks[i].Key < tasks[j].Key
	})
	return tasks
}
