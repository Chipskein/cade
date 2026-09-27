// Package queryplan turns a natural-language question into explicit
// filters using the local model. Rule-based parsing kept missing phrasings
// ("me retorne as mensagens com o marcos"); the model reads the question and
// a GBNF grammar guarantees its answer is well-formed. Rules only read
// questions with no word they do not know (rules.go).
package queryplan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/llm"
)

// Mode is what the user wants back.
type Mode int

const (
	// ModeAnswer: a question to answer from the most relevant events.
	ModeAnswer Mode = iota
	// ModeList: every event matching the filters.
	ModeList
	// ModeTasks: the task report ("quais tarefas fiz ontem?").
	ModeTasks
)

// TaskStatus narrows a task report to finished or unfinished tasks.
type TaskStatus int

const (
	AnyStatus TaskStatus = iota
	OnlyDone
	OnlyInProgress
)

// Plan holds only what the question states; most questions need no
// filters, and every field defaults to "unrestricted".
type Plan struct {
	Mode Mode
	// Period is the time expression as written ("ontem", "12/08"); the
	// caller resolves it to dates deterministically.
	Period   string
	Source   event.Source
	Criteria listing.Criteria
	// Topic is what the question is about ("redis"); it ranks events when
	// listing, so "páginas sobre redis" does not list every page.
	Topic string
	// TaskStatus only applies to ModeTasks.
	TaskStatus TaskStatus
	// ReadByRules is set when PlanByRules read the question without the
	// model.
	ReadByRules bool
}

// maxPlanTokens fits the JSON with a few names; the grammar ends it sooner.
const maxPlanTokens = 128

// Planner asks a grammar-constrained model for a Plan.
type Planner struct {
	generator llm.StructuredGenerator
}

// NewPlanner returns a Planner using generator.
//
//	plan, err := queryplan.NewPlanner(generator).Plan(ctx, "mensagens da Ana ontem")
func NewPlanner(generator llm.StructuredGenerator) Planner {
	return Planner{generator: generator}
}

// Plan interprets question, by rules when they cover every word, else with
// the model.
func (p Planner) Plan(ctx context.Context, question string) (Plan, error) {
	if plan, ok := PlanByRules(question); ok {
		return plan, nil
	}
	reply, err := p.generator.GenerateStructured(ctx, planMessages(question), maxPlanTokens, planGrammar)
	if err != nil {
		return Plan{}, fmt.Errorf("interpret question: %w", err)
	}
	plan, err := parsePlan(reply)
	if err != nil {
		return Plan{}, err
	}
	return guardPlan(plan, question), nil
}

// rawPlan mirrors the JSON the grammar allows.
type rawPlan struct {
	Tipo    string   `json:"tipo"`
	Periodo *string  `json:"periodo"`
	Fonte   *string  `json:"fonte"`
	Pessoas []string `json:"pessoas"`
	Direcao *string  `json:"direcao"`
	Assunto *string  `json:"assunto"`
	Status  *string  `json:"status"`
}

var (
	directions   = map[string]listing.Direction{"recebidas": listing.Received, "enviadas": listing.Sent}
	modes        = map[string]Mode{"responder": ModeAnswer, "listar": ModeList, "tarefas": ModeTasks}
	taskStatuses = map[string]TaskStatus{"concluidas": OnlyDone, "em_andamento": OnlyInProgress}
)

func parsePlan(reply string) (Plan, error) {
	var raw rawPlan
	if err := json.Unmarshal([]byte(reply), &raw); err != nil {
		return Plan{}, fmt.Errorf("parse plan %q, expected the JSON of planGrammar: %w", reply, err)
	}
	plan := Plan{
		Period: strings.TrimSpace(deref(raw.Periodo)), Source: event.Source(deref(raw.Fonte)),
		Topic:      strings.TrimSpace(deref(raw.Assunto)),
		Criteria:   listing.Criteria{Direction: directions[deref(raw.Direcao)], People: cleanNames(raw.Pessoas)},
		Mode:       modes[raw.Tipo],
		TaskStatus: taskStatuses[deref(raw.Status)],
	}
	return withoutForeignDirection(plan), nil
}

// withoutForeignDirection drops a direction the model attached to commits
// or pages, where sent/received has no meaning.
func withoutForeignDirection(plan Plan) Plan {
	if plan.Source != "" && plan.Source != event.SourceTeams {
		plan.Criteria.Direction = listing.AnyDirection
	}
	return plan
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cleanNames(names []string) []string {
	var cleaned []string
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
