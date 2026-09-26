package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/provenance"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

const (
	dayHeaderLayout   = "2006-01-02 (Mon)"
	clockLayout       = "15:04"
	fullStampLayout   = "2006-01-02 15:04"
	maxDescribedRunes = 110
)

func renderTimeline(out io.Writer, days timeline.DayRange, events []event.Event) {
	if len(events) == 0 {
		fmt.Fprintf(out, "Nenhum evento em %s.\n", days)
		return
	}
	fmt.Fprintf(out, "Timeline de %s — %d eventos\n", days, len(events))
	location := days.First.Location()
	var currentDay string
	for _, ev := range events {
		local := ev.Timestamp.In(location)
		if day := local.Format(dayHeaderLayout); day != currentDay {
			currentDay = day
			fmt.Fprintf(out, "\n── %s ──\n", day)
		}
		fmt.Fprintf(out, "%s  %-9s %s\n", local.Format(clockLayout), "["+string(ev.Source)+"]", describeEvent(ev))
	}
}

// describeEvent picks a one-line summary. Known sources get a richer line;
// any other source (e.g. a future Teams ingestor) falls back to the
// content's first line, so rendering never blocks a new source (CA11).
func describeEvent(ev event.Event) string {
	switch ev.Source {
	case event.SourceGit:
		return clipLine(ev.Headline()) + describeCommitOrigin(ev.Commit())
	case event.SourceBrowser:
		return describeVisit(ev.Visit())
	case event.SourceFile:
		return ev.File().Path
	case event.SourceTeams:
		return clipLine(ev.Headline()) + describeConversation(ev.Message())
	}
	return clipLine(ev.Headline())
}

func describeCommitOrigin(commit event.Commit) string {
	hash := commit.Hash
	if len(hash) > 8 {
		hash = hash[:8]
	}
	return fmt.Sprintf("  (%s %s)", filepath.Base(commit.Repository), hash)
}

func describeConversation(message event.Message) string {
	label := strings.TrimSpace(string(message.Kind) + " " + message.Conversation)
	if label == "" {
		return ""
	}
	return "  (" + clipLine(label) + ")"
}

func describeVisit(visit event.Visit) string {
	if visit.Title == "" {
		return clipLine(visit.URL)
	}
	return clipLine(visit.Title + " — " + visit.URL)
}

func clipLine(text string) string {
	runes := []rune(text)
	if len(runes) <= maxDescribedRunes {
		return text
	}
	return string(runes[:maxDescribedRunes-1]) + "…"
}

func renderAnswer(out io.Writer, answer rag.Answer, location *time.Location) {
	if !answer.Found {
		fmt.Fprintln(out, "Não encontrei informação sobre isso nos dados ingeridos.")
		return
	}
	fmt.Fprintf(out, "%s\n\n", answer.Text)
	renderSources(out, answer, location)
}

// renderSources lists the cited evidence, or all of it when the reply cited
// none, so the user can always check what the answer was based on.
func renderSources(out io.Writer, answer rag.Answer, location *time.Location) {
	title, numbers := "Fontes citadas:", answer.Cited
	if len(numbers) == 0 {
		title, numbers = "Eventos consultados (a resposta não citou nenhum):", allEvidenceNumbers(answer.Evidence)
	}
	fmt.Fprintln(out, title)
	for _, number := range numbers {
		renderEvidenceLine(out, number, answer.Evidence[number-1].Event, location)
	}
	renderUnknownCitations(out, answer.UnknownCitations)
}

// renderEvidenceLine adds the locator (full commit hash, Teams link) on a
// second line unless the summary already shows it (a page's URL, a path).
func renderEvidenceLine(out io.Writer, number int, ev event.Event, location *time.Location) {
	description := describeEvent(ev)
	fmt.Fprintf(out, "  [%d] %-9s %s  %s\n", number, "["+string(ev.Source)+"]",
		ev.Timestamp.In(location).Format(fullStampLayout), description)
	if locator := provenance.Of(ev).Locator; !strings.Contains(description, locator) {
		fmt.Fprintf(out, "      ↳ %s\n", locator)
	}
}

// renderUnknownCitations flags numbers the model cited that match no
// consulted event: the sentence next to them has no source.
func renderUnknownCitations(out io.Writer, unknown []int) {
	if len(unknown) == 0 {
		return
	}
	labels := make([]string, len(unknown))
	for i, number := range unknown {
		labels[i] = fmt.Sprintf("[%d]", number)
	}
	fmt.Fprintf(out, "Atenção: a resposta cita %s, que não corresponde a nenhum evento consultado; esse trecho não tem fonte.\n", strings.Join(labels, ", "))
}

func allEvidenceNumbers(evidence []storage.ScoredEvent) []int {
	numbers := make([]int, len(evidence))
	for i := range evidence {
		numbers[i] = i + 1
	}
	return numbers
}
