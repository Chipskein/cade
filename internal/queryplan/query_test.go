package queryplan

import (
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/timeline"
)

func TestResolveAppliesPlanAndDates(t *testing.T) {
	plan := Plan{Mode: ModeList, Period: "ontem", Source: event.SourceGit, Topic: "autenticação"}
	query := Resolve("commits de ontem sobre autenticação", plan, Overrides{}, suiteNow)
	if query.Mode != ModeList || query.Days.String() != "2026-09-25" || query.Source != event.SourceGit || query.SemanticText != "autenticação" {
		t.Fatalf("unexpected query %+v", query)
	}
}

func TestResolveOverridesWin(t *testing.T) {
	days := ResolvePeriod("hoje", "", suiteNow, timeline.DayFirst)
	plan := Plan{Period: "ontem", Source: event.SourceGit}
	query := Resolve("o que fiz ontem?", plan, Overrides{Source: event.SourceTeams, Days: days}, suiteNow)
	if query.Source != event.SourceTeams || query.Days.String() != "2026-09-26" {
		t.Fatalf("expected the flags to win, got %+v", query)
	}
}

func TestResolveIgnoreQuestionDropsPlanAndDates(t *testing.T) {
	plan := Plan{Mode: ModeList, Period: "ontem", Topic: "cache", Criteria: listing.Criteria{People: []string{"Ana"}}}
	query := Resolve("o que a Ana fez ontem?", plan, Overrides{IgnoreQuestion: true}, suiteNow)
	if query.Mode != ModeAnswer || query.Days != nil || !query.Criteria.IsEmpty() || query.SemanticText != "o que a Ana fez ontem?" {
		t.Fatalf("expected an unfiltered query, got %+v", query)
	}
}

func TestResolveTasksDefaultToToday(t *testing.T) {
	query := Resolve("quais tarefas finalizei?", Plan{Mode: ModeTasks}, Overrides{}, suiteNow)
	if query.Mode != ModeTasks || query.Days.String() != "2026-09-26" {
		t.Fatalf("expected today's task report, got %+v", query)
	}
}

// Regression guard for the cutoff: a bare topic sits farther from every
// event, so unscoped questions keep the whole text.
func TestSemanticTextUsesTopicOnlyWhenScoped(t *testing.T) {
	unscoped := Resolve("o que eu fiz sobre cache?", Plan{Topic: "cache"}, Overrides{}, suiteNow)
	scoped := Resolve("o que a Ana falou sobre cache?", Plan{Topic: "cache", Criteria: listing.Criteria{People: []string{"Ana"}}}, Overrides{}, suiteNow)
	if unscoped.SemanticText != "o que eu fiz sobre cache?" || scoped.SemanticText != "cache" {
		t.Fatalf("unexpected semantic texts %q / %q", unscoped.SemanticText, scoped.SemanticText)
	}
}

func TestIsScoped(t *testing.T) {
	days := ResolvePeriod("hoje", "", suiteNow, timeline.DayFirst)
	scoped := []Query{{Days: days}, {Source: event.SourceGit}, {Criteria: listing.Criteria{Direction: listing.Sent}}}
	for _, query := range scoped {
		if !query.IsScoped() {
			t.Errorf("expected %+v to be scoped", query)
		}
	}
	if (Query{}).IsScoped() {
		t.Error("expected an empty query to be unscoped")
	}
}

// Regression: "o que eu fiz?" counted colleagues' commits in a team repo.
func TestFirstPersonQuestionsKeepOwnCommits(t *testing.T) {
	cases := map[string]bool{"o que eu fiz ontem?": true, "quais commits fiz sobre auth?": true, "what did I work on?": true,
		"liste os commits de ontem": false, "o que o Rui commitou?": false}
	for question, expected := range cases {
		if got := Resolve(question, Plan{}, Overrides{}, suiteNow).OwnCommitsOnly; got != expected {
			t.Errorf("%q: OwnCommitsOnly = %v, expected %v", question, got, expected)
		}
	}
	withPerson := Resolve("o que eu pedi ao Rui?", Plan{Criteria: listing.Criteria{People: []string{"Rui"}}}, Overrides{}, suiteNow)
	if withPerson.OwnCommitsOnly {
		t.Fatal("a question naming a person is about that person too")
	}
}

func TestResolveReadsNumericDatesInTheGivenOrder(t *testing.T) {
	dayFirst := Resolve("o que fiz em 12/08?", Plan{}, Overrides{DateOrder: timeline.DayFirst}, suiteNow)
	monthFirst := Resolve("what did I do on 12/08?", Plan{}, Overrides{DateOrder: timeline.MonthFirst}, suiteNow)
	if dayFirst.Days.String() != "2026-08-12" || monthFirst.Days.String() != "2025-12-08" {
		t.Fatalf("expected 12 August in dmy and December 8 in mdy, got %v / %v", dayFirst.Days, monthFirst.Days)
	}
}
