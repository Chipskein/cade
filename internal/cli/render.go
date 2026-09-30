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

// timelineHeader is the timeline's first line: "Timeline de 2026-09-25 — 1 evento".
func timelineHeader(days timeline.DayRange, events int, language Language) string {
	if events == 0 {
		return fmt.Sprintf(language.pick("Nenhum evento em %s.\n", "No events on %s.\n"), days)
	}
	return fmt.Sprintf(language.pick("Timeline de %s — %s\n", "Timeline of %s — %s\n"), days, language.count(events, eventNoun))
}

func renderTimeline(out io.Writer, days timeline.DayRange, events []event.Event, language Language) {
	fmt.Fprint(out, timelineHeader(days, len(events), language))
	if len(events) == 0 {
		return
	}
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

// ImagePreviewer draws the image at path as terminal text, or returns "".
type ImagePreviewer func(path string) string

func renderAnswer(out io.Writer, answer rag.Answer, location *time.Location, language Language, preview ImagePreviewer) {
	if !answer.Found {
		fmt.Fprintln(out, language.pick("Não encontrei informação sobre isso nos dados ingeridos.", "I found nothing about this in the ingested data."))
		return
	}
	fmt.Fprintf(out, "%s\n\n", answer.Text)
	renderSources(out, answer, location, language, preview)
}

// renderSources lists the cited evidence, or all of it when the reply cited
// none, so the user can always check what the answer was based on.
func renderSources(out io.Writer, answer rag.Answer, location *time.Location, language Language, preview ImagePreviewer) {
	title, numbers := language.pick("Fontes citadas:", "Cited sources:"), answer.Cited
	if len(numbers) == 0 {
		title = language.pick("Eventos consultados (a resposta não citou nenhum):", "Events consulted (the answer cited none):")
		numbers = allEvidenceNumbers(answer.Evidence)
	}
	fmt.Fprintln(out, title)
	for _, number := range numbers {
		renderEvidenceLine(out, number, answer.Evidence[number-1], location, language)
		renderImagePreview(out, answer.Evidence[number-1].Event, preview)
	}
	renderUnknownCitations(out, answer.UnknownCitations, language)
}

// renderEvidenceLine adds the locator (full commit hash, Teams link) on a
// second line unless the summary already shows it (a page's URL, a path).
func renderEvidenceLine(out io.Writer, number int, hit storage.ScoredEvent, location *time.Location, language Language) {
	ev := hit.Event
	description := describeEvent(ev)
	fmt.Fprintf(out, "  [%d] %-9s %s  %s\n", number, "["+string(ev.Source)+"]",
		ev.Timestamp.In(location).Format(fullStampLayout), description)
	if note := rag.EvidenceNote(hit, location, noteWording(language)); note != "" {
		fmt.Fprintf(out, "      (%s)\n", note)
	}
	if locator := provenance.Of(ev).Locator; !strings.Contains(description, locator) {
		fmt.Fprintf(out, "      ↳ %s\n", locator)
	}
}

// renderImagePreview shows a cited image under its citation; a nil preview
// (output not a terminal) or an empty one leaves the citation unchanged.
func renderImagePreview(out io.Writer, ev event.Event, preview ImagePreviewer) {
	if preview == nil || ev.Source != event.SourceFile {
		return
	}
	if art := preview(ev.File().Path); art != "" {
		fmt.Fprintln(out, art)
	}
}

// renderUnknownCitations flags numbers the model cited that match no
// consulted event: the sentence next to them has no source.
func renderUnknownCitations(out io.Writer, unknown []int, language Language) {
	if len(unknown) == 0 {
		return
	}
	labels := make([]string, len(unknown))
	for i, number := range unknown {
		labels[i] = fmt.Sprintf("[%d]", number)
	}
	fmt.Fprintf(out, language.pick("Atenção: a resposta cita %s, que não corresponde a nenhum evento consultado; esse trecho não tem fonte.\n",
		"Warning: the answer cites %s, which matches no consulted event; that part has no source.\n"), strings.Join(labels, ", "))
}

// noteWording is the sources list's wording of rag.EvidenceNote.
func noteWording(language Language) rag.NoteWording {
	if language == English {
		return rag.EnglishWording
	}
	return rag.PromptWording
}

func allEvidenceNumbers(evidence []storage.ScoredEvent) []int {
	numbers := make([]int, len(evidence))
	for i := range evidence {
		numbers[i] = i + 1
	}
	return numbers
}
