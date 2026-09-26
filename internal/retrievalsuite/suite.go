// Package retrievalsuite measures retrieval quality: a synthetic corpus is
// ingested through the real pipeline and each question must bring back the
// events known to answer it. It pins how well search works, so a change to
// embeddings, filters or ranking shows up as a score instead of a feeling.
//
// Cases come in two sets over one corpus: thresholds are tuned on the
// calibration set only, and the test set, never looked at while tuning,
// is what the floors check.
package retrievalsuite

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
)

// Corpus is the history every case searches.
type Corpus struct {
	// Now anchors relative periods ("ontem") in the questions.
	Now    time.Time     `json:"now"`
	Events []CorpusEvent `json:"events"`
}

// CaseSet is one set of questions and, for the test set, its floors.
type CaseSet struct {
	// MinimumRecall and MinimumMRR are floors over the answerable cases;
	// MinimumRejection is the floor for questions nothing answers.
	MinimumRecall    float64 `json:"minimum_recall"`
	MinimumMRR       float64 `json:"minimum_mrr"`
	MinimumRejection float64 `json:"minimum_rejection"`
	Cases            []Case  `json:"cases"`
}

// Suite is a corpus with one case set, validated against each other.
type Suite struct {
	Corpus
	CaseSet
	// groups maps each event id to its group.
	groups map[string]string
	// distractors are extra unrelated events for the scale curve.
	distractors []event.Event
}

// CorpusEvent is stored as an event whose UID is ID. Group joins events
// that are the same thing for the user (15 visits to one page, versions of
// one file); it defaults to the id, and cases name groups.
type CorpusEvent struct {
	ID       string         `json:"id"`
	Group    string         `json:"group,omitempty"`
	Source   event.Source   `json:"source"`
	At       time.Time      `json:"at"`
	Content  string         `json:"content"`
	Metadata event.Metadata `json:"metadata"`
}

func (e CorpusEvent) group() string {
	if e.Group != "" {
		return e.Group
	}
	return e.ID
}

// Case is a question with the filters the planner would read from it, so
// the suite measures retrieval alone, and the groups that answer it (none:
// nothing in the corpus does).
type Case struct {
	Question  string       `json:"question"`
	Period    string       `json:"period"`
	Source    event.Source `json:"source"`
	People    []string     `json:"people"`
	Direction string       `json:"direction"`
	Topic     string       `json:"topic"`
	Relevant  []string     `json:"relevant"`
}

var directions = map[string]listing.Direction{"": listing.AnyDirection, "recebidas": listing.Received, "enviadas": listing.Sent}

// Query resolves the case as `cade ask` resolves a planned question.
func (c Case) Query(now time.Time) queryplan.Query {
	plan := queryplan.Plan{Period: c.Period, Source: c.Source, Topic: c.Topic,
		Criteria: listing.Criteria{Direction: directions[c.Direction], People: c.People}}
	return queryplan.Resolve(c.Question, plan, queryplan.Overrides{}, now)
}

// LoadCorpus reads the shared corpus.
//
//	corpus, err := retrievalsuite.LoadCorpus(file)
func LoadCorpus(reader io.Reader) (Corpus, error) {
	var corpus Corpus
	if err := json.NewDecoder(reader).Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode retrieval corpus, expected {now, events}: %w", err)
	}
	if corpus.Now.IsZero() || len(corpus.Events) == 0 {
		return Corpus{}, fmt.Errorf("retrieval corpus has now=%v and %d events; expected both", corpus.Now, len(corpus.Events))
	}
	return corpus, nil
}

// LoadCases reads a calibration or test case set.
func LoadCases(reader io.Reader) (CaseSet, error) {
	var set CaseSet
	if err := json.NewDecoder(reader).Decode(&set); err != nil {
		return CaseSet{}, fmt.Errorf("decode retrieval cases, expected {minimum_*, cases}: %w", err)
	}
	if len(set.Cases) == 0 {
		return CaseSet{}, fmt.Errorf("retrieval case set has no cases, expected at least one")
	}
	return set, nil
}

// NewSuite pairs a corpus with a case set; a relevant group missing from
// the corpus is a typo, not a retrieval miss.
func NewSuite(corpus Corpus, set CaseSet) (Suite, error) {
	suite := Suite{Corpus: corpus, CaseSet: set, groups: map[string]string{}}
	known := map[string]bool{}
	for _, ev := range corpus.Events {
		suite.groups[ev.ID] = ev.group()
		known[ev.group()] = true
	}
	for i, suiteCase := range set.Cases {
		if err := validateCase(suiteCase, known); err != nil {
			return Suite{}, fmt.Errorf("retrieval case %d %q: %w", i+1, suiteCase.Question, err)
		}
	}
	return suite, nil
}

func validateCase(suiteCase Case, groups map[string]bool) error {
	if _, known := directions[suiteCase.Direction]; !known {
		return fmt.Errorf("direction %q, expected \"\", \"recebidas\" or \"enviadas\"", suiteCase.Direction)
	}
	for _, group := range suiteCase.Relevant {
		if !groups[group] {
			return fmt.Errorf("relevant group %q is not in the corpus", group)
		}
	}
	return nil
}

// events converts the corpus to stored events, plus any distractors.
func (s Suite) events() []event.Event {
	events := make([]event.Event, 0, len(s.Events)+len(s.distractors))
	for _, corpusEvent := range s.Events {
		events = append(events, event.Event{UID: corpusEvent.ID, Source: corpusEvent.Source, Timestamp: corpusEvent.At,
			Content: corpusEvent.Content, Metadata: corpusEvent.Metadata})
	}
	return append(events, s.distractors...)
}

// groupOf maps a retrieved event id to its group; unknown ids (synthetic
// distractors) are their own group.
func (s Suite) groupOf(id string) string {
	if group, found := s.groups[id]; found {
		return group
	}
	return id
}
