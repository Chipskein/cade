package queryplan

import (
	"testing"

	"github.com/chipskein/cade/internal/listing"
)

func directionPlan(direction listing.Direction, people ...string) Plan {
	return Plan{Criteria: listing.Criteria{Direction: direction, People: people}}
}

func TestGuardKeepsSupportedDirections(t *testing.T) {
	cases := map[string]Plan{
		"O que a Ana me passou ontem":           directionPlan(listing.Received, "Ana"),
		"mensagens que recebi da Ana":           directionPlan(listing.Received, "Ana"),
		"qual foi a última mensagem do Leandro": directionPlan(listing.Received, "Leandro"),
		"mensagens pro Willian hoje":            directionPlan(listing.Sent, "Willian"),
		"mensagens que mandei hoje":             directionPlan(listing.Sent),
	}
	for question, plan := range cases {
		direction := plan.Criteria.Direction
		if got := guardPlan(plan, question); got.Criteria.Direction != direction {
			t.Errorf("guardPlan(%q) dropped a supported direction", question)
		}
	}
}

// Regression: the model returned directions for "com X" questions.
func TestGuardDropsInventedDirections(t *testing.T) {
	cases := map[string]listing.Direction{
		"conversas com o Edilson sobre a Bigfertil": listing.Sent,
		"resumo do que conversei com a Ianne":       listing.Sent,
		"o Vitor falou algo sobre os PRs?":          listing.Received,
	}
	for question, direction := range cases {
		if got := guardPlan(directionPlan(direction), question); got.Criteria.Direction != listing.AnyDirection {
			t.Errorf("guardPlan(%q) kept an unsupported direction %v", question, direction)
		}
	}
}

// Regression: "essa semana" came back as "semana passada".
func TestGuardDropsPeriodNotInQuestion(t *testing.T) {
	plan := guardPlan(Plan{Period: "semana passada"}, "resumo do que conversei com a Ianne essa semana")
	kept := guardPlan(Plan{Period: "últimos 3 dias"}, "conversas nos ultimos 3 dias")
	if plan.Period != "" || kept.Period != "últimos 3 dias" {
		t.Fatalf("expected rewritten period dropped and literal one kept, got %q / %q", plan.Period, kept.Period)
	}
}

func TestGuardEnglishCues(t *testing.T) {
	kept := map[string]Plan{
		"what did Ana send me yesterday? she sent me a file": directionPlan(listing.Received, "Ana"),
		"messages from Ana today":                            directionPlan(listing.Received, "Ana"),
		"messages I sent to Rui":                             directionPlan(listing.Sent, "Rui"),
		"what did I tell the team":                           directionPlan(listing.Sent),
	}
	for question, plan := range kept {
		if got := guardPlan(plan, question); got.Criteria.Direction != plan.Criteria.Direction {
			t.Errorf("guardPlan(%q) dropped a supported direction", question)
		}
	}
	if got := guardPlan(directionPlan(listing.Received, "Edilson"), "chats with Edilson about the invoice"); got.Criteria.Direction != listing.AnyDirection {
		t.Error("\"with X\" must not keep a direction")
	}
}

func TestGuardTaskReport(t *testing.T) {
	cases := []struct {
		question string
		in       Plan
		mode     Mode
		status   TaskStatus
	}{
		{"quais tarefas finalizei essa semana?", Plan{Mode: ModeTasks, TaskStatus: OnlyDone}, ModeTasks, OnlyDone},
		{"which tasks are still in progress?", Plan{Mode: ModeTasks, TaskStatus: OnlyInProgress}, ModeTasks, OnlyInProgress},
		{"tarefas que não finalizei ontem", Plan{Mode: ModeTasks, TaskStatus: OnlyDone}, ModeTasks, AnyStatus},
		{"tarefas que não finalizei ontem", Plan{Mode: ModeTasks, TaskStatus: OnlyInProgress}, ModeTasks, OnlyInProgress},
		{"quais tarefas fiz ontem?", Plan{Mode: ModeTasks, TaskStatus: OnlyDone}, ModeTasks, AnyStatus},
		{"o que eu fiz ontem?", Plan{Mode: ModeTasks}, ModeAnswer, AnyStatus},
		{"mensagens que finalizei", Plan{Mode: ModeList, TaskStatus: OnlyDone}, ModeList, AnyStatus},
	}
	for _, c := range cases {
		got := guardPlan(c.in, c.question)
		if got.Mode != c.mode || got.TaskStatus != c.status {
			t.Errorf("guardPlan(%q): expected mode %d status %d, got %d %d", c.question, c.mode, c.status, got.Mode, got.TaskStatus)
		}
	}
}
