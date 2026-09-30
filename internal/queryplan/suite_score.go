package queryplan

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/textnorm"
)

// Mismatch is one field the planner got wrong.
type Mismatch struct {
	Field    Field
	Expected string
	Got      string
}

// CaseResult is how one suite question was interpreted.
type CaseResult struct {
	Question   string
	Mismatches []Mismatch
	// ByRules is set when the rules read the question, not the model.
	ByRules bool
}

// ScoreCase compares plan, resolved as `cade ask` resolves it, with what
// suiteCase expects.
//
//	result := queryplan.ScoreCase(suiteCase, plan, suite.Now)
func ScoreCase(suiteCase SuiteCase, plan Plan, now time.Time) CaseResult {
	expected, got := expectedValues(suiteCase.Expect), observedValues(plan, suiteCase, now)
	result := CaseResult{Question: suiteCase.Question, ByRules: plan.ReadByRules}
	for _, field := range scoredFields {
		if !fieldMatches(field, expected[field], got[field]) {
			result.Mismatches = append(result.Mismatches, Mismatch{Field: field, Expected: expected[field], Got: got[field]})
		}
	}
	return result
}

func expectedValues(expect ExpectedPlan) map[Field]string {
	mode := expect.Mode
	if mode == "" {
		mode = keyOf(modes, ModeAnswer)
	}
	return map[Field]string{FieldMode: mode, FieldDays: expect.Days, FieldSource: expect.Source, FieldPeople: peopleKey(expect.People),
		FieldDirection: expect.Direction, FieldTopic: expect.Topic, FieldStatus: expect.Status}
}

func observedValues(plan Plan, suiteCase SuiteCase, now time.Time) map[Field]string {
	query, days := Resolve(suiteCase.Question, plan, Overrides{DateOrder: suiteCase.dateOrder()}, now), ""
	if query.Days != nil {
		days = query.Days.String()
	}
	return map[Field]string{FieldMode: keyOf(modes, query.Mode), FieldDays: days, FieldSource: string(query.Source),
		FieldPeople: peopleKey(query.Criteria.People), FieldDirection: keyOf(directions, query.Criteria.Direction),
		FieldTopic: query.Topic, FieldStatus: keyOf(taskStatuses, query.TaskStatus)}
}

// keyOf is the planner word for value, or "" for the unrestricted value.
func keyOf[V comparable](values map[string]V, value V) string {
	for key, candidate := range values {
		if candidate == value {
			return key
		}
	}
	return ""
}

// peopleKey compares names as retrieval does, ignoring order, case,
// accents and spelling variants: "Ana, joão" equals "João, ana".
func peopleKey(people []string) string {
	folded := make([]string, len(people))
	for i, person := range people {
		folded[i] = listing.NameKey(person)
	}
	sort.Strings(folded)
	return strings.Join(folded, ", ")
}

// fieldMatches compares exactly, except topics: the model may write "o
// deploy" for "deploy", so one containing the other counts.
func fieldMatches(field Field, expected, got string) bool {
	if field != FieldTopic || expected == "" || got == "" {
		return expected == got
	}
	expected, got = textnorm.Fold(expected), textnorm.Fold(got)
	return strings.Contains(got, expected) || strings.Contains(expected, got)
}

// Scoreboard accumulates per-field accuracy over a suite run.
type Scoreboard struct {
	Cases    int
	Correct  map[Field]int
	Failures []CaseResult
	// RuleCases and RuleFailures count the questions read without the
	// model, to measure how many skip it and whether the rules err.
	RuleCases    int
	RuleFailures int
}

// NewScoreboard returns an empty scoreboard.
func NewScoreboard() *Scoreboard {
	return &Scoreboard{Correct: map[Field]int{}}
}

// Add records one case.
func (s *Scoreboard) Add(result CaseResult) {
	s.Cases++
	wrong := map[Field]bool{}
	for _, mismatch := range result.Mismatches {
		wrong[mismatch.Field] = true
	}
	for _, field := range scoredFields {
		if !wrong[field] {
			s.Correct[field]++
		}
	}
	if len(result.Mismatches) > 0 {
		s.Failures = append(s.Failures, result)
	}
	s.addRuleCase(result)
}

func (s *Scoreboard) addRuleCase(result CaseResult) {
	if !result.ByRules {
		return
	}
	s.RuleCases++
	if len(result.Mismatches) > 0 {
		s.RuleFailures++
	}
}

// Accuracy is the share of cases with field right, from 0 to 1.
func (s *Scoreboard) Accuracy(field Field) float64 {
	if s.Cases == 0 {
		return 0
	}
	return float64(s.Correct[field]) / float64(s.Cases)
}

// Interval is the 95% Wilson score interval of field's accuracy. With a
// few dozen cases one miss moves the accuracy by several points, so floors
// are checked against the lower bound, not the point estimate.
func (s *Scoreboard) Interval(field Field) (float64, float64) {
	return wilsonInterval(s.Correct[field], s.Cases)
}

// wilsonZ is the normal quantile for a 95% interval.
const wilsonZ = 1.96

func wilsonInterval(successes, trials int) (float64, float64) {
	if trials == 0 {
		return 0, 1
	}
	n, p := float64(trials), float64(successes)/float64(trials)
	center := (p + wilsonZ*wilsonZ/(2*n)) / (1 + wilsonZ*wilsonZ/n)
	margin := wilsonZ * math.Sqrt(p*(1-p)/n+wilsonZ*wilsonZ/(4*n*n)) / (1 + wilsonZ*wilsonZ/n)
	return max(0, center-margin), min(1, center+margin)
}

// BelowMinimum lists the fields whose interval's lower bound fell under
// the suite's floor.
func (s *Scoreboard) BelowMinimum(minimums map[Field]float64) []Field {
	var below []Field
	for _, field := range scoredFields {
		low, _ := s.Interval(field)
		if minimum, enforced := minimums[field]; enforced && low < minimum {
			below = append(below, field)
		}
	}
	return below
}

// WriteReport prints the per-field scores, then each wrongly read question.
func (s *Scoreboard) WriteReport(out io.Writer) {
	fmt.Fprintf(out, "%d perguntas, %d totalmente corretas\n", s.Cases, s.Cases-len(s.Failures))
	fmt.Fprintf(out, "  lidas só por regras: %d, com %d erradas\n", s.RuleCases, s.RuleFailures)
	for _, field := range scoredFields {
		low, high := s.Interval(field)
		fmt.Fprintf(out, "  %-10s %3d/%d  %3.0f%%  [%3.0f%%–%3.0f%%]\n", field, s.Correct[field], s.Cases, 100*s.Accuracy(field), 100*low, 100*high)
	}
	for _, failure := range s.Failures {
		fmt.Fprintf(out, "✗ %s\n", failure.Question)
		for _, mismatch := range failure.Mismatches {
			fmt.Fprintf(out, "    %s: esperado %q, veio %q\n", mismatch.Field, mismatch.Expected, mismatch.Got)
		}
	}
}

// CaseScored observes a suite run, e.g. to show progress; done counts from 1.
type CaseScored func(done, total int, result CaseResult)

// RunSuite interprets every question with planner and scores it; onCase,
// when non-nil, is called after each question.
//
//	board, err := queryplan.RunSuite(ctx, queryplan.NewPlanner(generator), suite, nil)
func RunSuite(ctx context.Context, planner Planner, suite Suite, onCase CaseScored) (*Scoreboard, error) {
	board := NewScoreboard()
	for i, suiteCase := range suite.Cases {
		plan, err := planner.Plan(ctx, suiteCase.Question)
		if err != nil {
			return nil, fmt.Errorf("plan %q: %w", suiteCase.Question, err)
		}
		result := ScoreCase(suiteCase, plan, suite.Now)
		board.Add(result)
		if onCase != nil {
			onCase(i+1, len(suite.Cases), result)
		}
	}
	return board, nil
}
