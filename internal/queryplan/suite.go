package queryplan

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/timeline"
)

// A Suite pins how representative questions must be interpreted, so a
// prompt or model change that degrades planning lowers a measured score
// instead of passing silently.
type Suite struct {
	// Now anchors relative periods ("ontem") so expected dates are fixed.
	Now time.Time `json:"now"`
	// MinimumAccuracy is the lowest accepted share of correct cases per
	// field, from 0 to 1; fields left out are measured but not enforced.
	MinimumAccuracy map[Field]float64 `json:"minimum_accuracy"`
	Cases           []SuiteCase       `json:"cases"`
}

// SuiteCase is one question and the plan it should produce.
type SuiteCase struct {
	Question string `json:"question"`
	// DateOrder is "dmy" or "mdy", as ui.date_order; required when the
	// question has a numeric date, so its expected day never depends on a
	// default.
	DateOrder string       `json:"date_order"`
	Expect    ExpectedPlan `json:"expect"`
}

// dateOrder is the case's order; LoadSuite already rejected invalid ones.
func (c SuiteCase) dateOrder() timeline.DateOrder {
	order, _ := timeline.ParseDateOrder(c.DateOrder)
	return order
}

// ExpectedPlan uses the planner's own vocabulary ("listar", "recebidas");
// an omitted field expects "unrestricted" and an omitted mode "responder".
type ExpectedPlan struct {
	Mode      string   `json:"mode"`
	Days      string   `json:"days"`
	Source    string   `json:"source"`
	People    []string `json:"people"`
	Direction string   `json:"direction"`
	Topic     string   `json:"topic"`
	Status    string   `json:"status"`
}

// Field names one scored part of a plan.
type Field string

const (
	FieldMode      Field = "mode"
	FieldDays      Field = "days"
	FieldSource    Field = "source"
	FieldPeople    Field = "people"
	FieldDirection Field = "direction"
	FieldTopic     Field = "topic"
	FieldStatus    Field = "status"
)

// scoredFields fixes the order fields are reported in.
var scoredFields = []Field{FieldMode, FieldDays, FieldSource, FieldPeople, FieldDirection, FieldTopic, FieldStatus}

var sources = map[string]bool{"": true, "git": true, "browser": true, "file": true, "teams": true}

// LoadSuite reads and validates a suite, so a typo in an expected value
// fails loudly instead of counting as a model mistake.
//
//	suite, err := queryplan.LoadSuite(file)
func LoadSuite(reader io.Reader) (Suite, error) {
	var suite Suite
	if err := json.NewDecoder(reader).Decode(&suite); err != nil {
		return Suite{}, fmt.Errorf("decode plan suite, expected {now, minimum_accuracy, cases}: %w", err)
	}
	if suite.Now.IsZero() || len(suite.Cases) == 0 {
		return Suite{}, fmt.Errorf("plan suite has now=%v and %d cases, expected a timestamp and at least one case", suite.Now, len(suite.Cases))
	}
	if err := validateMinimums(suite.MinimumAccuracy); err != nil {
		return Suite{}, err
	}
	for i, suiteCase := range suite.Cases {
		if err := validateCase(suiteCase); err != nil {
			return Suite{}, fmt.Errorf("plan suite case %d: %w", i+1, err)
		}
	}
	return suite, nil
}

func validateMinimums(minimums map[Field]float64) error {
	for field, minimum := range minimums {
		if !isScoredField(field) || minimum < 0 || minimum > 1 {
			return fmt.Errorf("minimum_accuracy %q=%v, expected one of %v with a value from 0 to 1", field, minimum, scoredFields)
		}
	}
	return nil
}

func isScoredField(field Field) bool {
	for _, known := range scoredFields {
		if field == known {
			return true
		}
	}
	return false
}

func validateCase(suiteCase SuiteCase) error {
	if suiteCase.Question == "" {
		return fmt.Errorf("empty question, expected the text to interpret")
	}
	if err := validateDateOrder(suiteCase); err != nil {
		return err
	}
	for _, check := range expectationChecks(suiteCase.Expect) {
		if !check.valid {
			return fmt.Errorf("%q: %s %q is not a planner value", suiteCase.Question, check.name, check.value)
		}
	}
	return nil
}

func validateDateOrder(suiteCase SuiteCase) error {
	if suiteCase.DateOrder == "" && timeline.HasNumericDate(suiteCase.Question) {
		return fmt.Errorf("%q has a numeric date but no date_order, expected \"dmy\" or \"mdy\"", suiteCase.Question)
	}
	if _, err := timeline.ParseDateOrder(suiteCase.DateOrder); suiteCase.DateOrder != "" && err != nil {
		return fmt.Errorf("%q: %w", suiteCase.Question, err)
	}
	return nil
}

type expectationCheck struct {
	name, value string
	valid       bool
}

func expectationChecks(expect ExpectedPlan) []expectationCheck {
	return []expectationCheck{
		{"mode", expect.Mode, expect.Mode == "" || hasKey(modes, expect.Mode)},
		{"source", expect.Source, sources[expect.Source]},
		{"direction", expect.Direction, expect.Direction == "" || hasKey(directions, expect.Direction)},
		{"status", expect.Status, expect.Status == "" || hasKey(taskStatuses, expect.Status)},
	}
}

func hasKey[V any](values map[string]V, key string) bool {
	_, found := values[key]
	return found
}
