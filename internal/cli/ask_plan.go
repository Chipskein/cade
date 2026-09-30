package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/timeline"
)

// resolveAskQuery reads the question's filters, unless --no-filters, and
// resolves them with the flags, which win.
func (env commandEnv) resolveAskQuery(ctx context.Context, models *askModels, text string, filters askFlags, session *askSession) (queryplan.Query, error) {
	overrides, err := askOverrides(filters, env.toolkit.Now(), env.dateOrder)
	if err != nil {
		return queryplan.Query{}, err
	}
	// Without a home, a "~/..." folder stays unexpanded and matches nothing,
	// which the empty answer shows.
	overrides.Home, _ = env.toolkit.HomeDir()
	plan, err := env.interpret(ctx, models, text, overrides.IgnoreQuestion, session)
	if err != nil {
		return queryplan.Query{}, err
	}
	query := queryplan.Resolve(text, plan, overrides, env.toolkit.Now())
	env.logger.Debug("question resolved", "mode", planCodes.modes[query.Mode], "source", query.Source,
		"period", describeDays(query.Days), "folder", query.Folder, "topic", query.Topic, "semantic_text", query.SemanticText)
	return env.announceQuery(query, session), nil
}

func askOverrides(filters askFlags, now time.Time, order timeline.DateOrder) (queryplan.Overrides, error) {
	days, err := parseOptionalDays(*filters.from, *filters.to, now)
	return queryplan.Overrides{Source: event.Source(*filters.source), Days: days, IgnoreQuestion: *filters.noFilters, DateOrder: order}, err
}

// interpret reads the filters by rules when they cover the question, else
// with the model; a model failure only costs the filters, but a model that
// does not load stops the command.
func (env commandEnv) interpret(ctx context.Context, models *askModels, text string, disabled bool, session *askSession) (queryplan.Plan, error) {
	if disabled {
		return queryplan.Plan{}, nil
	}
	if plan, ok := queryplan.PlanByRules(text); ok {
		env.logger.Debug("question read by rules", "question_chars", len(text))
		return plan, nil
	}
	generator, err := models.loadedGenerator()
	if err != nil {
		return queryplan.Plan{}, err
	}
	session.status.show(env.language.pick("Interpretando pergunta…", "Reading the question…"))
	plan, err := queryplan.NewPlanner(generator).Plan(ctx, text)
	if err != nil {
		env.logger.Warn("question interpretation failed; answering without filters", "error", err.Error())
		return queryplan.Plan{}, nil
	}
	return plan, nil
}

// announceQuery prints what was understood, so a wrong reading is visible
// and can be overridden with flags.
func (env commandEnv) announceQuery(query queryplan.Query, session *askSession) queryplan.Query {
	session.status.clear()
	fmt.Fprintf(env.stderr, env.language.pick("Entendi: %s\n", "Understood: %s\n"), describeQuery(query, env.language))
	return query
}

// queryLabels word describeQuery's parts in one language.
type queryLabels struct {
	modes      map[queryplan.Mode]string
	directions map[listing.Direction]string
	statuses   map[queryplan.TaskStatus]string
	people     string
	folder     string
	topic      string
	noFilters  string
}

// planCodes are the plan's values in `ask --json` and the debug logs: the
// JSON is an interface, so they stay the Portuguese labels whatever the
// language.
var planCodes = portugueseQueryLabels

var portugueseQueryLabels = queryLabels{
	modes:      map[queryplan.Mode]string{queryplan.ModeAnswer: "responder", queryplan.ModeList: "listar", queryplan.ModeTasks: "tarefas"},
	directions: map[listing.Direction]string{listing.Received: "recebidas", listing.Sent: "enviadas"},
	statuses:   map[queryplan.TaskStatus]string{queryplan.OnlyDone: "PR aberto", queryplan.OnlyInProgress: "em andamento"},
	people:     "pessoas: ", folder: "pasta: ", topic: "assunto: ", noFilters: "sem filtros",
}

var englishQueryLabels = queryLabels{
	modes:      map[queryplan.Mode]string{queryplan.ModeAnswer: "answer", queryplan.ModeList: "list", queryplan.ModeTasks: "tasks"},
	directions: map[listing.Direction]string{listing.Received: "received", listing.Sent: "sent"},
	statuses:   map[queryplan.TaskStatus]string{queryplan.OnlyDone: "PR opened", queryplan.OnlyInProgress: "in progress"},
	people:     "people: ", folder: "folder: ", topic: "topic: ", noFilters: "no filters",
}

func describeQuery(query queryplan.Query, language Language) string {
	labels := portugueseQueryLabels
	if language == English {
		labels = englishQueryLabels
	}
	parts := []string{labels.modes[query.Mode]}
	parts = appendIf(parts, string(query.Source), string(query.Source))
	parts = appendIf(parts, describeDays(query.Days), describeDays(query.Days))
	people := strings.Join(query.Criteria.People, ", ")
	parts = appendIf(parts, people, labels.people+people)
	parts = appendIf(parts, labels.directions[query.Criteria.Direction], labels.directions[query.Criteria.Direction])
	parts = appendIf(parts, query.Folder, labels.folder+query.Folder)
	parts = appendIf(parts, query.Topic, labels.topic+query.Topic)
	parts = appendIf(parts, labels.statuses[query.TaskStatus], labels.statuses[query.TaskStatus])
	if len(parts) == 1 {
		parts = append(parts, labels.noFilters)
	}
	return strings.Join(parts, " · ")
}

func appendIf(parts []string, condition, part string) []string {
	if condition == "" {
		return parts
	}
	return append(parts, part)
}

func describeDays(days *timeline.DayRange) string {
	if days == nil {
		return ""
	}
	return days.String()
}
