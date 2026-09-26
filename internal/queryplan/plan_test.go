package queryplan

import (
	"context"
	"errors"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/llm"
)

// FakeStructuredGenerator returns Reply and records the grammar it got.
type FakeStructuredGenerator struct {
	Reply       string
	FailWith    error
	LastGrammar string
	LastPrompt  []llm.ChatMessage
}

func (f *FakeStructuredGenerator) GenerateStructured(_ context.Context, messages []llm.ChatMessage, _ int, grammar string) (string, error) {
	f.LastGrammar, f.LastPrompt = grammar, messages
	return f.Reply, f.FailWith
}

func TestPlanParsesListingWithFilters(t *testing.T) {
	generator := &FakeStructuredGenerator{Reply: `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Ana", " "], "direcao": "recebidas", "assunto": "deploy"}`}
	plan, err := NewPlanner(generator).Plan(context.Background(), "mensagens que recebi da Ana ontem")
	expected := Plan{Mode: ModeList, Period: "ontem", Source: event.SourceTeams,
		Criteria: listing.Criteria{Direction: listing.Received, People: []string{"Ana"}}}
	if err != nil || plan.Mode != expected.Mode || plan.Period != "ontem" || plan.Source != event.SourceTeams ||
		plan.Criteria.Direction != listing.Received || len(plan.Criteria.People) != 1 || plan.Criteria.People[0] != "Ana" || plan.Topic != "deploy" {
		t.Fatalf("expected %+v, got %+v (err %v)", expected, plan, err)
	}
}

func TestPlanWithoutFilters(t *testing.T) {
	generator := &FakeStructuredGenerator{Reply: `{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null}`}
	plan, err := NewPlanner(generator).Plan(context.Background(), "o que fiz sobre cache?")
	if err != nil || plan.Mode != ModeAnswer || plan.Period != "" || plan.Source != "" || !plan.Criteria.IsEmpty() {
		t.Fatalf("expected an unrestricted answer plan, got %+v (err %v)", plan, err)
	}
}

func TestPlanUsesGrammarAndQuestion(t *testing.T) {
	generator := &FakeStructuredGenerator{Reply: `{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null}`}
	NewPlanner(generator).Plan(context.Background(), "pergunta final")
	last := generator.LastPrompt[len(generator.LastPrompt)-1]
	if generator.LastGrammar != planGrammar || last.Content != "pergunta final" || last.Role != llm.RoleUser {
		t.Fatalf("expected the grammar and the question as last user turn, got %+v", last)
	}
}

func TestPlanWrapsGeneratorError(t *testing.T) {
	generator := &FakeStructuredGenerator{FailWith: errors.New("out of memory")}
	if _, err := NewPlanner(generator).Plan(context.Background(), "x"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParsePlanRejectsInvalidJSON(t *testing.T) {
	if _, err := parsePlan("{truncated"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestPlanMessagesIncludeExamples(t *testing.T) {
	messages := planMessages("q")
	if len(messages) != 2+2*len(planExamples) || messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected system + example pairs + question, got %d messages", len(messages))
	}
}

// The examples must themselves parse, or they would teach a broken shape.
func TestPlanExamplesAreValid(t *testing.T) {
	for _, example := range planExamples {
		if _, err := parsePlan(example.plan); err != nil {
			t.Errorf("example %q: %v", example.question, err)
		}
	}
}

func TestPlanDropsDirectionForNonMessageSources(t *testing.T) {
	plan, _ := parsePlan(`{"tipo": "listar", "periodo": "hoje", "fonte": "browser", "pessoas": [], "direcao": "recebidas", "assunto": "redis"}`)
	if plan.Criteria.Direction != listing.AnyDirection {
		t.Fatalf("expected no direction for browser, got %v", plan.Criteria.Direction)
	}
}
