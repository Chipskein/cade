package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/timeline"
)

// resolveAskQuery reads the question's filters with the model, unless
// --no-filters, and resolves them with the flags, which win.
func (env commandEnv) resolveAskQuery(ctx context.Context, generator llm.StructuredGenerator, text string, filters askFlags, session *askSession) (queryplan.Query, error) {
	overrides, err := askOverrides(filters, env.toolkit.Now())
	if err != nil {
		return queryplan.Query{}, err
	}
	plan := env.interpret(ctx, generator, text, overrides.IgnoreQuestion, session)
	query := queryplan.Resolve(text, plan, overrides, env.toolkit.Now())
	env.logger.Debug("question resolved", "mode", modeLabels[query.Mode], "source", query.Source,
		"period", describeDays(query.Days), "topic", query.Topic, "semantic_text", query.SemanticText)
	return env.announceQuery(query, session), nil
}

func askOverrides(filters askFlags, now time.Time) (queryplan.Overrides, error) {
	days, err := parseOptionalDays(*filters.from, *filters.to, now)
	return queryplan.Overrides{Source: event.Source(*filters.source), Days: days, IgnoreQuestion: *filters.noFilters}, err
}

// interpret asks the model for filters; a failure only costs the filters.
func (env commandEnv) interpret(ctx context.Context, generator llm.StructuredGenerator, text string, disabled bool, session *askSession) queryplan.Plan {
	if disabled {
		return queryplan.Plan{}
	}
	session.status.show("Interpretando pergunta…")
	plan, err := queryplan.NewPlanner(generator).Plan(ctx, text)
	if err != nil {
		env.logger.Warn("question interpretation failed; answering without filters", "error", err.Error())
		return queryplan.Plan{}
	}
	return plan
}

// announceQuery prints what was understood, so a wrong reading is visible
// and can be overridden with flags.
func (env commandEnv) announceQuery(query queryplan.Query, session *askSession) queryplan.Query {
	session.status.clear()
	fmt.Fprintf(env.stderr, "Entendi: %s\n", describeQuery(query))
	return query
}

var (
	directionLabels = map[listing.Direction]string{listing.Received: "recebidas", listing.Sent: "enviadas"}
	modeLabels      = map[queryplan.Mode]string{queryplan.ModeAnswer: "responder", queryplan.ModeList: "listar", queryplan.ModeTasks: "tarefas"}
	statusFilters   = map[queryplan.TaskStatus]string{queryplan.OnlyDone: "concluídas", queryplan.OnlyInProgress: "em andamento"}
)

func describeQuery(query queryplan.Query) string {
	parts := []string{modeLabels[query.Mode]}
	parts = appendIf(parts, string(query.Source), string(query.Source))
	parts = appendIf(parts, describeDays(query.Days), describeDays(query.Days))
	people := strings.Join(query.Criteria.People, ", ")
	parts = appendIf(parts, people, "pessoas: "+people)
	parts = appendIf(parts, directionLabels[query.Criteria.Direction], directionLabels[query.Criteria.Direction])
	parts = appendIf(parts, query.Topic, "assunto: "+query.Topic)
	parts = appendIf(parts, statusFilters[query.TaskStatus], statusFilters[query.TaskStatus])
	if len(parts) == 1 {
		parts = append(parts, "sem filtros")
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
