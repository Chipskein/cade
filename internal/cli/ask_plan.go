package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/timeline"
)

// askPlan is the resolved interpretation of a question.
type askPlan struct {
	mode     queryplan.Mode
	question rag.Question
	topic    string
}

// resolveAskPlan combines the model's reading of the question with the
// flags (which win) and resolves dates deterministically.
func (env commandEnv) resolveAskPlan(ctx context.Context, generator llm.StructuredGenerator, text string, filters askFlags, session *askSession) (askPlan, error) {
	plan := env.interpret(ctx, generator, text, *filters.noFilters, session)
	days, err := env.resolveDays(text, plan.Period, filters)
	if err != nil {
		return askPlan{}, err
	}
	source := event.Source(*filters.source)
	if source == "" {
		source = plan.Source
	}
	resolved := askPlan{mode: plan.Mode, topic: plan.Topic,
		question: rag.Question{Text: text, Source: source, Days: days, Criteria: plan.Criteria}}
	return env.announcePlan(listingNeedsPeriod(resolved), session), nil
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

// resolveDays prefers the flags, then a date the deterministic parser finds
// in the question, then the model's period expression.
func (env commandEnv) resolveDays(text, modelPeriod string, filters askFlags) (*timeline.DayRange, error) {
	days, err := parseOptionalDays(*filters.from, *filters.to, env.toolkit.Now())
	if err != nil || days != nil || *filters.noFilters {
		return days, err
	}
	for _, source := range []string{text, modelPeriod} {
		if detected, found := timeline.DetectDayRange(source, env.toolkit.Now()); source != "" && found {
			return &detected, nil
		}
	}
	return nil, nil
}

// listingNeedsPeriod turns a period-less listing into an answer: listing
// every event ever is never what "as mensagens do Marcos" means.
func listingNeedsPeriod(plan askPlan) askPlan {
	if plan.mode == queryplan.ModeList && plan.question.Days == nil {
		plan.mode = queryplan.ModeAnswer
	}
	return plan
}

// announcePlan prints what was understood, so a wrong reading is visible
// and can be overridden with flags.
func (env commandEnv) announcePlan(plan askPlan, session *askSession) askPlan {
	session.status.clear()
	fmt.Fprintf(env.stderr, "Entendi: %s\n", describePlan(plan))
	return plan
}

var directionLabels = map[listing.Direction]string{listing.Received: "recebidas", listing.Sent: "enviadas"}

func describePlan(plan askPlan) string {
	parts := []string{map[queryplan.Mode]string{queryplan.ModeAnswer: "responder", queryplan.ModeList: "listar"}[plan.mode]}
	question := plan.question
	parts = appendIf(parts, string(question.Source), string(question.Source))
	if question.Days != nil {
		parts = append(parts, question.Days.String())
	}
	parts = appendIf(parts, strings.Join(question.Criteria.People, ", "), "pessoas: "+strings.Join(question.Criteria.People, ", "))
	parts = appendIf(parts, directionLabels[question.Criteria.Direction], directionLabels[question.Criteria.Direction])
	parts = appendIf(parts, plan.topic, "assunto: "+plan.topic)
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
