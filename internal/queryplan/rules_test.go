package queryplan

import (
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
)

// TestPlanByRulesOnShippedSuite is the rules' accuracy check: every suite
// question they read must come out fully right, so skipping the model
// never costs accuracy. The share they read is logged.
func TestPlanByRulesOnShippedSuite(t *testing.T) {
	suite := loadShippedSuite(t)
	read := 0
	for _, suiteCase := range suite.Cases {
		plan, ok := PlanByRules(suiteCase.Question)
		if !ok {
			continue
		}
		read++
		if result := ScoreCase(suiteCase, plan, suite.Now); len(result.Mismatches) > 0 {
			t.Errorf("rules misread %q: %+v", suiteCase.Question, result.Mismatches)
		}
	}
	t.Logf("rules read %d of %d suite questions", read, len(suite.Cases))
	if read < len(suite.Cases)/4 {
		t.Errorf("rules read %d of %d suite questions, expected at least a quarter", read, len(suite.Cases))
	}
}

func TestPlanByRulesReadsPlainQuestions(t *testing.T) {
	cases := map[string]Plan{
		"liste os commits de ontem":          {Mode: ModeList, Period: "ontem", Source: event.SourceGit},
		"o que eu fiz?":                      {Mode: ModeAnswer},
		"mensagens que recebi hoje":          {Mode: ModeList, Period: "hoje", Source: event.SourceTeams, Criteria: listing.Criteria{Direction: listing.Received}},
		"resumo das mensagens de ontem":      {Mode: ModeAnswer, Period: "ontem", Source: event.SourceTeams},
		"tarefas que não finalizei":          {Mode: ModeTasks, TaskStatus: OnlyInProgress},
		"which tasks did I finish today?":    {Mode: ModeTasks, Period: "today", TaskStatus: OnlyDone},
		"quais tarefas estão com PR aberto?": {Mode: ModeTasks, TaskStatus: OnlyDone},
		"show everything from yesterday":     {Mode: ModeList, Period: "yesterday"},
		"liste os commits de 20/09":          {Mode: ModeList, Period: "20/09", Source: event.SourceGit},
		"list the commits from 9/20":         {Mode: ModeList, Period: "9/20", Source: event.SourceGit},
	}
	for question, expected := range cases {
		expected.ReadByRules = true
		plan, ok := PlanByRules(question)
		if !ok || !samePlan(plan, expected) {
			t.Errorf("PlanByRules(%q) = %+v (%v), expected %+v", question, plan, ok, expected)
		}
	}
}

func samePlan(a, b Plan) bool {
	return a.Mode == b.Mode && a.Period == b.Period && a.Source == b.Source && a.Criteria.Direction == b.Criteria.Direction &&
		len(a.Criteria.People) == len(b.Criteria.People) && a.Topic == b.Topic && a.TaskStatus == b.TaskStatus && a.ReadByRules == b.ReadByRules
}

// Each of these needs the model: a name, a topic, a number, two sources,
// a direction outside messages, or an "o que" over a source.
func TestPlanByRulesLeavesTheRestToTheModel(t *testing.T) {
	for _, question := range []string{
		"o que a Carla me passou ontem?",
		"commits sobre autenticação",
		"qual a causa do erro 502?",
		"commits e mensagens de ontem",
		"commits que recebi ontem",
		"tarefas nos commits de hoje",
		"o que tem nas mensagens de ontem?",
		"liste os commits do Atlas",
	} {
		if plan, ok := PlanByRules(question); ok {
			t.Errorf("PlanByRules(%q) = %+v, expected the model to read it", question, plan)
		}
	}
}

func TestPlannerSkipsTheModelWhenRulesRead(t *testing.T) {
	generator := &FakeStructuredGenerator{}
	plan, err := NewPlanner(generator).Plan(t.Context(), "liste os commits de ontem")
	if err != nil || !plan.ReadByRules || generator.LastGrammar != "" {
		t.Fatalf("plan %+v, error %v, grammar %q: expected rules and no model call", plan, err, generator.LastGrammar)
	}
}
