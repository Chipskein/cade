package cli

import (
	"encoding/json"
	"io"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/provenance"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/tasks"
)

// askReport is `cade ask --json`: the resolved plan and, for the mode it
// ran, the result with a reference to every record behind it. Fields of
// other modes are null.
type askReport struct {
	Question string                 `json:"question"`
	Plan     planReport             `json:"plan"`
	Answer   *answerReport          `json:"answer"`
	Events   []provenance.Reference `json:"events"`
	Tasks    []taskReport           `json:"tasks"`
}

type planReport struct {
	Mode         string   `json:"mode"`
	Source       string   `json:"source,omitempty"`
	Period       string   `json:"period,omitempty"`
	People       []string `json:"people,omitempty"`
	Direction    string   `json:"direction,omitempty"`
	Topic        string   `json:"topic,omitempty"`
	TaskStatus   string   `json:"task_status,omitempty"`
	SemanticText string   `json:"semantic_text,omitempty"`
}

type answerReport struct {
	Found            bool             `json:"found"`
	Text             string           `json:"text"`
	UnknownCitations []int            `json:"unknown_citations"`
	Evidence         []evidenceReport `json:"evidence"`
}

// evidenceReport is one event given to the model, numbered as in the
// prompt and the reply's [n] citations.
type evidenceReport struct {
	Number   int     `json:"n"`
	Cited    bool    `json:"cited"`
	Distance float64 `json:"distance"`
	// Occurrences counts this event plus the repeats folded into it
	// (visits to the same page, versions of the same file).
	Occurrences int       `json:"occurrences"`
	LatestAt    time.Time `json:"latest_at"`
	// Chunk (from 1) of Chunks matched; ExcerptStart and ExcerptEnd are its
	// byte offsets in the event's text, the part given to the model.
	Chunk        int `json:"chunk"`
	Chunks       int `json:"chunks"`
	ExcerptStart int `json:"excerpt_start"`
	ExcerptEnd   int `json:"excerpt_end"`
	// Untrusted marks text that gives the assistant orders (a prompt
	// injection); the model was told not to follow it.
	Untrusted bool `json:"untrusted"`
	provenance.Reference
}

type taskReport struct {
	Key         string                 `json:"key"`
	Title       string                 `json:"title"`
	Status      string                 `json:"status"`
	Involvement string                 `json:"involvement"`
	PRs         []prReport             `json:"prs"`
	Evidence    []provenance.Reference `json:"evidence"`
}

type prReport struct {
	Ref      string    `json:"ref"`
	Title    string    `json:"title,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	Link     string    `json:"link"`
}

var (
	taskStatusCodes  = map[tasks.Status]string{tasks.Done: "pr_aberto", tasks.InProgress: "em_andamento"}
	involvementCodes = map[tasks.Involvement]string{tasks.Mine: "sua", tasks.Consulted: "consultada", tasks.MentionedByOthers: "citada_por_outros"}
	linkCodes        = map[tasks.Link]string{tasks.LinkExact: "exata", tasks.LinkProbable: "provavel"}
)

func writeAskReport(out io.Writer, report askReport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func newAskReport(query queryplan.Query) askReport {
	return askReport{Question: query.Question, Plan: planReport{
		Mode: planCodes.modes[query.Mode], Source: string(query.Source), Period: describeDays(query.Days),
		People: query.Criteria.People, Direction: planCodes.directions[query.Criteria.Direction], Topic: query.Topic,
		TaskStatus: planCodes.statuses[query.TaskStatus], SemanticText: query.SemanticText,
	}}
}

func answerReportOf(answer rag.Answer) *answerReport {
	cited := map[int]bool{}
	for _, number := range answer.Cited {
		cited[number] = true
	}
	report := &answerReport{Found: answer.Found, Text: answer.Text, UnknownCitations: nonNil(answer.UnknownCitations), Evidence: []evidenceReport{}}
	for i, hit := range answer.Evidence {
		report.Evidence = append(report.Evidence, evidenceReport{Number: i + 1, Cited: cited[i+1], Distance: hit.Distance,
			Occurrences: hit.Repeats + 1, LatestAt: latestOccurrence(hit), Chunk: hit.Chunk.Ordinal + 1, Chunks: max(hit.ChunkCount, 1),
			ExcerptStart: hit.Chunk.Start, ExcerptEnd: excerptEnd(hit), Untrusted: rag.AddressesAssistant(hit.Event), Reference: provenance.Of(hit.Event)})
	}
	return report
}

func eventReferences(events []event.Event) []provenance.Reference {
	return nonNil(provenance.All(events))
}

func taskReports(list []tasks.Task) []taskReport {
	reports := []taskReport{}
	for _, task := range list {
		reports = append(reports, taskReport{Key: task.Key, Title: task.Title, Status: taskStatusCodes[task.Status],
			Involvement: involvementCodes[task.Involvement], PRs: prReports(task.PRs), Evidence: eventReferences(task.Events)})
	}
	return reports
}

func prReports(prs []tasks.LinkedPR) []prReport {
	reports := []prReport{}
	for _, pr := range prs {
		reports = append(reports, prReport{Ref: pr.Ref.Key(), Title: pr.Title, OpenedAt: pr.OpenedAt, Link: linkCodes[pr.Link]})
	}
	return reports
}

// nonNil makes empty lists encode as [] instead of null, so null keeps
// meaning "not this mode".
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

func latestOccurrence(hit storage.ScoredEvent) time.Time {
	if hit.LatestAt.IsZero() {
		return hit.Event.Timestamp
	}
	return hit.LatestAt
}

// excerptEnd covers the whole text for single-chunk events found without
// chunk offsets (a store that does not split).
func excerptEnd(hit storage.ScoredEvent) int {
	if hit.Chunk.End == 0 {
		return len(hit.Event.Content)
	}
	return hit.Chunk.End
}
