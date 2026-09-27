package queryplan

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/timeline"
)

const shippedSuitePath = "../../testdata/queries/plan.json"

var suiteNow = time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("BRT", -3*3600))

func loadShippedSuite(t *testing.T) Suite {
	t.Helper()
	file, err := os.Open(shippedSuitePath)
	if err != nil {
		t.Fatalf("open %s: %v", shippedSuitePath, err)
	}
	defer file.Close()
	suite, err := LoadSuite(file)
	if err != nil {
		t.Fatalf("load %s: %v", shippedSuitePath, err)
	}
	return suite
}

func TestShippedSuiteIsValid(t *testing.T) {
	if suite := loadShippedSuite(t); len(suite.Cases) < 30 {
		t.Fatalf("expected at least 30 cases, got %d", len(suite.Cases))
	}
}

func TestLoadSuiteRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		`{"now": "2026-09-26T10:00:00Z", "cases": [{"question": "x", "expect": {"mode": "listagem"}}]}`:    "listagem",
		`{"now": "2026-09-26T10:00:00Z", "cases": [{"question": "x", "expect": {"source": "slack"}}]}`:     "slack",
		`{"now": "2026-09-26T10:00:00Z", "minimum_accuracy": {"mood": 0.5}, "cases": [{"question": "x"}]}`: "mood",
		`{"now": "2026-09-26T10:00:00Z", "minimum_accuracy": {"mode": 1.5}, "cases": [{"question": "x"}]}`: "1.5",
		`{"now": "2026-09-26T10:00:00Z", "cases": []}`:                                                     "0 cases",
		`{"now": "2026-09-26T10:00:00Z", "cases": [{"question": ""}]}`:                                     "empty question",
	}
	for raw, offending := range cases {
		if _, err := LoadSuite(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), offending) {
			t.Errorf("expected an error naming %q, got %v", offending, err)
		}
	}
}

func TestScoreCaseAcceptsMatchingPlan(t *testing.T) {
	suiteCase := SuiteCase{Question: "o que o Marcos me pediu ontem sobre o deploy?",
		Expect: ExpectedPlan{Days: "2026-09-25", People: []string{"Marcos"}, Direction: "recebidas", Topic: "deploy"}}
	plan := Plan{Period: "ontem", Topic: "o deploy", Criteria: listing.Criteria{Direction: listing.Received, People: []string{"marcos"}}}
	if result := ScoreCase(suiteCase, plan, suiteNow); len(result.Mismatches) != 0 {
		t.Fatalf("expected no mismatch, got %+v", result.Mismatches)
	}
}

func TestScoreCaseReportsEachWrongField(t *testing.T) {
	suiteCase := SuiteCase{Question: "conversas com o Everton", Expect: ExpectedPlan{Mode: "listar", Source: "teams", People: []string{"Everton"}}}
	plan := Plan{Mode: ModeAnswer, Source: "teams", Criteria: listing.Criteria{Direction: listing.Sent, People: []string{"Everton"}}}
	result := ScoreCase(suiteCase, plan, suiteNow)
	expected := []Mismatch{{FieldMode, "listar", "responder"}, {FieldDirection, "", "enviadas"}}
	if len(result.Mismatches) != 2 || result.Mismatches[0] != expected[0] || result.Mismatches[1] != expected[1] {
		t.Fatalf("expected %+v, got %+v", expected, result.Mismatches)
	}
}

func TestFieldMatchesTopicByContainment(t *testing.T) {
	if !fieldMatches(FieldTopic, "migração do banco", "a Migracao do banco") || fieldMatches(FieldTopic, "redis", "") || fieldMatches(FieldTopic, "redis", "kafka") {
		t.Fatal("unexpected topic comparison")
	}
	if fieldMatches(FieldSource, "git", "gi") {
		t.Fatal("expected other fields to compare exactly")
	}
}

func TestPeopleKeyIgnoresOrderCaseAndAccents(t *testing.T) {
	if peopleKey([]string{"João", "ana"}) != peopleKey([]string{"Ana", "joao"}) {
		t.Fatal("expected equal keys")
	}
}

func TestKeyOfUnrestrictedValueIsEmpty(t *testing.T) {
	if keyOf(directions, listing.AnyDirection) != "" || keyOf(modes, ModeList) != "listar" {
		t.Fatal("unexpected planner words")
	}
}

func TestScoreboardAccuracyAndMinimums(t *testing.T) {
	board := NewScoreboard()
	board.Add(CaseResult{Question: "a"})
	board.Add(CaseResult{Question: "b", Mismatches: []Mismatch{{FieldMode, "listar", "responder"}}})
	if board.Accuracy(FieldMode) != 0.5 || board.Accuracy(FieldDays) != 1 || len(board.Failures) != 1 {
		t.Fatalf("unexpected scoreboard %+v", board)
	}
	below := board.BelowMinimum(map[Field]float64{FieldMode: 0.8, FieldDays: 0.3})
	if len(below) != 1 || below[0] != FieldMode {
		t.Fatalf("expected only mode below its floor, got %v", below)
	}
}

// Floors are checked against the lower bound: 2/2 correct is not proof of
// 100% accuracy.
func TestWilsonInterval(t *testing.T) {
	low, high := wilsonInterval(45, 50)
	if math.Abs(low-0.7864) > 0.001 || math.Abs(high-0.9565) > 0.001 {
		t.Fatalf("expected [0.786, 0.957], got [%.4f, %.4f]", low, high)
	}
	if low, high := wilsonInterval(2, 2); low > 0.35 || high != 1 {
		t.Fatalf("expected a wide interval for 2/2, got [%.2f, %.2f]", low, high)
	}
	if low, high := wilsonInterval(0, 0); low != 0 || high != 1 {
		t.Fatalf("expected [0, 1] without trials, got [%.2f, %.2f]", low, high)
	}
}

func TestWriteReportListsFailures(t *testing.T) {
	board := NewScoreboard()
	board.Add(CaseResult{Question: "quais tickets?", Mismatches: []Mismatch{{FieldStatus, "em_andamento", ""}}})
	var out strings.Builder
	board.WriteReport(&out)
	if !strings.Contains(out.String(), "status       0/1    0%  [  0%– 79%]") || !strings.Contains(out.String(), `status: esperado "em_andamento", veio ""`) {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
}

func TestRunSuiteScoresPlannerReplies(t *testing.T) {
	generator := &FakeStructuredGenerator{Reply: `{"tipo": "listar", "periodo": "ontem", "fonte": "git", "pessoas": [], "direcao": null, "assunto": null, "status": null}`}
	suite := Suite{Now: suiteNow, Cases: []SuiteCase{
		{Question: "liste meus commits de ontem", Expect: ExpectedPlan{Mode: "listar", Days: "2026-09-25", Source: "git"}},
		{Question: "o que a Ana fez?"},
	}}
	board, err := RunSuite(context.Background(), NewPlanner(generator), suite, nil)
	if err != nil || board.Cases != 2 || len(board.Failures) != 1 || board.Failures[0].Question != "o que a Ana fez?" {
		t.Fatalf("expected only the second case to fail, got %+v (err %v)", board, err)
	}
	if board.RuleCases != 1 || board.RuleFailures != 0 {
		t.Fatalf("expected the first case read by rules, got %d read and %d wrong", board.RuleCases, board.RuleFailures)
	}
}

func TestRunSuiteStopsOnPlannerError(t *testing.T) {
	generator := &FakeStructuredGenerator{FailWith: errors.New("out of memory")}
	suite := Suite{Now: suiteNow, Cases: []SuiteCase{{Question: "x"}}}
	if _, err := RunSuite(context.Background(), NewPlanner(generator), suite, nil); err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Fatalf("expected an error naming the question, got %v", err)
	}
}

func TestResolveModeNeedsPeriodToList(t *testing.T) {
	today := ResolvePeriod("hoje", "", suiteNow)
	if ResolveMode(ModeList, nil) != ModeAnswer || ResolveMode(ModeList, today) != ModeList || ResolveMode(ModeTasks, nil) != ModeTasks {
		t.Fatal("expected only a period-less listing to become an answer")
	}
}

func TestScoreCaseUsesResolvedMode(t *testing.T) {
	suiteCase := SuiteCase{Question: "em que site li sobre redis?", Expect: ExpectedPlan{Source: "browser", Topic: "redis"}}
	plan := Plan{Mode: ModeList, Source: "browser", Topic: "redis"}
	if result := ScoreCase(suiteCase, plan, suiteNow); len(result.Mismatches) != 0 {
		t.Fatalf("expected a period-less listing to count as an answer, got %+v", result.Mismatches)
	}
}

func TestResolvePeriodPrefersQuestionDate(t *testing.T) {
	fromQuestion := ResolvePeriod("o que fiz ontem?", "semana passada", suiteNow)
	fromModel := ResolvePeriod("o que fiz?", "hoje", suiteNow)
	if fromQuestion.String() != "2026-09-25" || fromModel.String() != "2026-09-26" || ResolvePeriod("o que fiz?", "", suiteNow) != nil {
		t.Fatalf("unexpected periods %v / %v", fromQuestion, fromModel)
	}
}

func TestPeopleKeyMergesSpellingVariants(t *testing.T) {
	if peopleKey([]string{"wilian"}) != peopleKey([]string{"Willian"}) {
		t.Fatal("expected spelling variants to score as the same person")
	}
}

func TestRunSuiteReportsEachCase(t *testing.T) {
	generator := &FakeStructuredGenerator{Reply: `{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": null}`}
	suite := Suite{Now: suiteNow, Cases: []SuiteCase{{Question: "a"}, {Question: "b"}}}
	var seen []string
	RunSuite(context.Background(), NewPlanner(generator), suite, func(done, total int, result CaseResult) {
		seen = append(seen, fmt.Sprintf("%d/%d %s", done, total, result.Question))
	})
	if strings.Join(seen, ",") != "1/2 a,2/2 b" {
		t.Fatalf("unexpected progress %v", seen)
	}
}

// Expected periods must be what the deterministic parser reads from the
// question, so a typo in a hand-written date fails here, not as a model
// mistake.
func TestShippedSuitePeriodsMatchTheParser(t *testing.T) {
	suite := loadShippedSuite(t)
	for _, suiteCase := range suite.Cases {
		resolved := describeDays(ResolvePeriod(suiteCase.Question, "", suite.Now))
		expected := suiteCase.Expect.Days
		if expected != "" && suiteCase.Expect.Mode == "tarefas" && resolved == "" {
			expected = ""
		}
		if resolved != expected {
			t.Errorf("%q: parser reads %q, suite expects %q", suiteCase.Question, resolved, suiteCase.Expect.Days)
		}
	}
}

func describeDays(days *timeline.DayRange) string {
	if days == nil {
		return ""
	}
	return days.String()
}
