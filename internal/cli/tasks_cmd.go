package cli

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/tasks"
	"github.com/chipskein/cade/internal/timeline"
)

// prHistorySpan is how far back to look for when a task's PR was opened: a
// a task with an opened PR last month still counts as PR opened today.
const prHistorySpan = 90 * 24 * time.Hour

// runTasks reports the tasks worked on in a period: `cade tasks [--all] [DATA [FIM]]`.
func runTasks(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("tasks", env.stderr, env.language)
	showAll := flags.Bool("all", false, env.language.pick("também lista tarefas que só apareceram em mensagens de outras pessoas",
		"also list tasks that only appeared in other people's messages"))
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	days, err := parseTasksDays(positional, env.toolkit.Now(), env.language)
	if err != nil {
		return err
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		patterns, err := compileTaskPatterns(cfg.Tasks.TaskURLPatterns)
		if err != nil {
			return err
		}
		report, err := buildTaskReport(ctx, store, patterns, days)
		if err != nil {
			return err
		}
		renderTaskReport(env.stdout, days, report, *showAll, env.language)
		return nil
	})
}

func parseTasksDays(args []string, now time.Time, language Language) (timeline.DayRange, error) {
	if len(args) == 0 {
		return timeline.ParseDayRange("hoje", "", now)
	}
	return parseTimelineDays(args, now, language)
}

func compileTaskPatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("tasks.task_url_patterns: %q is not a valid regex, expected one with capture groups: %w", pattern, err)
		}
		compiled = append(compiled, expression)
	}
	return compiled, nil
}

func buildTaskReport(ctx context.Context, store storage.EventStore, patterns []*regexp.Regexp, days timeline.DayRange) (tasks.Report, error) {
	period, err := store.EventsBetween(ctx, days.Start(), days.End())
	if err != nil {
		return tasks.Report{}, err
	}
	history, err := store.EventsBetween(ctx, days.Start().Add(-prHistorySpan), days.End())
	if err != nil {
		return tasks.Report{}, err
	}
	// A colleague's commit citing a task is not the user's work on it.
	return tasks.NewBuilder(patterns).Build(ownCommitsOnly(period), ownCommitsOnly(history), days.End()), nil
}
