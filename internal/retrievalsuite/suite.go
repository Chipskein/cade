// Package retrievalsuite measures retrieval quality: a synthetic corpus is
// ingested through the real pipeline and each question must bring back the
// events known to answer it. It pins how well search works, so a change to
// embeddings, filters or ranking shows up as a score instead of a feeling.
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

// Suite is a corpus plus questions over it.
type Suite struct {
	// Now anchors relative periods ("ontem") in the questions.
	Now time.Time `json:"now"`
	// MinimumRecall and MinimumMRR are floors over the answerable cases;
	// MinimumRejection is the floor for questions nothing answers.
	MinimumRecall    float64       `json:"minimum_recall"`
	MinimumMRR       float64       `json:"minimum_mrr"`
	MinimumRejection float64       `json:"minimum_rejection"`
	Events           []CorpusEvent `json:"events"`
	Cases            []Case        `json:"cases"`
}

// CorpusEvent is stored as an event whose UID is ID.
type CorpusEvent struct {
	ID       string         `json:"id"`
	Source   event.Source   `json:"source"`
	At       time.Time      `json:"at"`
	Content  string         `json:"content"`
	Metadata event.Metadata `json:"metadata"`
}

// Case is a question with the filters the planner would read from it, so
// the suite measures retrieval alone, and the events that answer it
// (none: nothing in the corpus does).
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

// Load reads and validates a suite; an expected id missing from the corpus
// is a typo, not a retrieval miss.
//
//	suite, err := retrievalsuite.Load(file)
func Load(reader io.Reader) (Suite, error) {
	var suite Suite
	if err := json.NewDecoder(reader).Decode(&suite); err != nil {
		return Suite{}, fmt.Errorf("decode retrieval suite, expected {now, events, cases}: %w", err)
	}
	if suite.Now.IsZero() || len(suite.Events) == 0 || len(suite.Cases) == 0 {
		return Suite{}, fmt.Errorf("retrieval suite has now=%v, %d events, %d cases; expected all three", suite.Now, len(suite.Events), len(suite.Cases))
	}
	ids := map[string]bool{}
	for _, ev := range suite.Events {
		ids[ev.ID] = true
	}
	for i, suiteCase := range suite.Cases {
		if err := validateCase(suiteCase, ids); err != nil {
			return Suite{}, fmt.Errorf("retrieval suite case %d %q: %w", i+1, suiteCase.Question, err)
		}
	}
	return suite, nil
}

func validateCase(suiteCase Case, ids map[string]bool) error {
	if _, known := directions[suiteCase.Direction]; !known {
		return fmt.Errorf("direction %q, expected \"\", \"recebidas\" or \"enviadas\"", suiteCase.Direction)
	}
	for _, id := range suiteCase.Relevant {
		if !ids[id] {
			return fmt.Errorf("relevant id %q is not in the corpus", id)
		}
	}
	return nil
}

// events converts the corpus to stored events.
func (s Suite) events() []event.Event {
	events := make([]event.Event, len(s.Events))
	for i, corpusEvent := range s.Events {
		events[i] = event.Event{UID: corpusEvent.ID, Source: corpusEvent.Source, Timestamp: corpusEvent.At,
			Content: corpusEvent.Content, Metadata: corpusEvent.Metadata}
	}
	return events
}
