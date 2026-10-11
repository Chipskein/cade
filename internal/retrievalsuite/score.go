package retrievalsuite

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// CaseResult is what one question retrieved, in rank order, as groups.
type CaseResult struct {
	Question  string
	Relevant  []string
	Retrieved []string
	// Distances match Retrieved; they show where max_distance would cut.
	Distances []float64
	// Gated questions are the ones the distance gates judge: no exact
	// filter and no identifier. Calibration looks only at those.
	Gated bool
	// PendingIssue is the case's: set, the result is reported apart and
	// left out of every average (see Case.PendingIssue).
	PendingIssue string
}

// Repeats counts retrieved slots taken by a group already retrieved: the
// same page visited again, another version of the same file.
func (r CaseResult) Repeats() int {
	seen := map[string]bool{}
	repeats := 0
	for _, group := range r.Retrieved {
		repeats += boolToInt(seen[group])
		seen[group] = true
	}
	return repeats
}

// Answerable reports whether some corpus event answers the question.
func (r CaseResult) Answerable() bool {
	return len(r.Relevant) > 0
}

// Recall is the share of relevant events retrieved.
func (r CaseResult) Recall() float64 {
	if !r.Answerable() {
		return 0
	}
	return float64(len(r.Relevant)-len(r.Missing())) / float64(len(r.Relevant))
}

// ReciprocalRank is 1/rank of the first relevant event, 0 if none came back.
func (r CaseResult) ReciprocalRank() float64 {
	for i, id := range r.Retrieved {
		if slices.Contains(r.Relevant, id) {
			return 1 / float64(i+1)
		}
	}
	return 0
}

// Missing lists the relevant events not retrieved.
func (r CaseResult) Missing() []string {
	var missing []string
	for _, id := range r.Relevant {
		if !slices.Contains(r.Retrieved, id) {
			missing = append(missing, id)
		}
	}
	return missing
}

// Passed: answerable cases retrieve every relevant event; the others
// retrieve nothing, so the model is never handed unrelated evidence.
func (r CaseResult) Passed() bool {
	if r.Answerable() {
		return len(r.Missing()) == 0
	}
	return len(r.Retrieved) == 0
}

// Scoreboard aggregates a run.
type Scoreboard struct {
	Results []CaseResult
}

// Counted are the results the averages and floors use: all but pending.
func (s Scoreboard) Counted() []CaseResult {
	return s.partition(false)
}

// Pending are the results of cases waiting for an issue.
func (s Scoreboard) Pending() []CaseResult {
	return s.partition(true)
}

func (s Scoreboard) partition(pending bool) []CaseResult {
	var results []CaseResult
	for _, result := range s.Results {
		if (result.PendingIssue != "") == pending {
			results = append(results, result)
		}
	}
	return results
}

// MeanRecall and MRR average over answerable cases.
func (s Scoreboard) MeanRecall() float64 {
	return s.meanOverAnswerable(CaseResult.Recall)
}

func (s Scoreboard) MRR() float64 {
	return s.meanOverAnswerable(CaseResult.ReciprocalRank)
}

func (s Scoreboard) meanOverAnswerable(metric func(CaseResult) float64) float64 {
	total, count := 0.0, 0
	for _, result := range s.Counted() {
		if result.Answerable() {
			total, count = total+metric(result), count+1
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// Redundancy is the share of all retrieved slots that repeat a group.
func (s Scoreboard) Redundancy() float64 {
	repeats, slots := 0, 0
	for _, result := range s.Counted() {
		repeats, slots = repeats+result.Repeats(), slots+len(result.Retrieved)
	}
	if slots == 0 {
		return 0
	}
	return float64(repeats) / float64(slots)
}

// Rejection is the share of unanswerable cases that retrieved nothing.
func (s Scoreboard) Rejection() float64 {
	rejected, count := 0, 0
	for _, result := range s.Counted() {
		if !result.Answerable() {
			count++
			rejected += boolToInt(result.Passed())
		}
	}
	if count == 0 {
		return 1
	}
	return float64(rejected) / float64(count)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// BelowMinimum names the metrics under the set's floors.
func (s Scoreboard) BelowMinimum(suite CaseSet) []string {
	var below []string
	checks := []struct {
		name           string
		value, minimum float64
	}{{"recall", s.MeanRecall(), suite.MinimumRecall}, {"mrr", s.MRR(), suite.MinimumMRR}, {"rejection", s.Rejection(), suite.MinimumRejection}}
	for _, check := range checks {
		if check.value < check.minimum {
			below = append(below, fmt.Sprintf("%s %.2f < %.2f", check.name, check.value, check.minimum))
		}
	}
	if suite.MaximumRedundancy != nil && s.Redundancy() > *suite.MaximumRedundancy {
		below = append(below, fmt.Sprintf("redundancy %.2f > %.2f", s.Redundancy(), *suite.MaximumRedundancy))
	}
	return below
}

// WriteReport prints the metrics, then each failed case with what it
// missed or wrongly retrieved, then the pending cases.
func (s Scoreboard) WriteReport(out io.Writer) {
	counted := s.Counted()
	fmt.Fprintf(out, "%d perguntas, %d corretas\n", len(counted), passedCount(counted))
	fmt.Fprintf(out, "  recall      %.2f\n  mrr         %.2f\n  rejeição    %.2f\n  redundância %.2f\n", s.MeanRecall(), s.MRR(), s.Rejection(), s.Redundancy())
	for _, result := range counted {
		if !result.Passed() {
			fmt.Fprintf(out, "✗ %s\n    %s\n", result.Question, failureDetail(result))
		}
	}
	s.writePending(out)
}

// writePending reports each pending case; one that passes is flagged, so
// its pending_issue is removed and it starts counting.
func (s Scoreboard) writePending(out io.Writer) {
	pending := s.Pending()
	if len(pending) == 0 {
		return
	}
	fmt.Fprintf(out, "%d pendentes, fora das médias:\n", len(pending))
	for _, result := range pending {
		if result.Passed() {
			fmt.Fprintf(out, "✓ %s (%s)\n    passou: remova pending_issue\n", result.Question, result.PendingIssue)
			continue
		}
		fmt.Fprintf(out, "⧗ %s (%s)\n    %s\n", result.Question, result.PendingIssue, failureDetail(result))
	}
}

func passedCount(results []CaseResult) int {
	passed := 0
	for _, result := range results {
		passed += boolToInt(result.Passed())
	}
	return passed
}

func failureDetail(result CaseResult) string {
	if !result.Answerable() {
		return "deveria vir vazio, veio: " + strings.Join(result.Retrieved, ", ")
	}
	return fmt.Sprintf("faltou: %s; veio: %s", strings.Join(result.Missing(), ", "), strings.Join(result.Retrieved, ", "))
}
