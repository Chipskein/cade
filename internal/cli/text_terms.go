package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/textnorm"
)

// A name that matches no sender or conversation (a client read as a
// person, a nickname) still restricts the result as text: dropping it
// listed every message of the day for "mensagens que enviei pro avilla".

func reportNamesAsText(out io.Writer, unknown []string) {
	if len(unknown) > 0 {
		fmt.Fprintf(out, "Nenhuma pessoa com esse nome; buscando como texto: %s\n", strings.Join(unknown, ", "))
	}
}

// eventsMentioningAll keeps the events whose text, sender or conversation
// contains every term, ignoring case and accents.
func eventsMentioningAll(events []event.Event, terms []string) []event.Event {
	if len(terms) == 0 {
		return events
	}
	var kept []event.Event
	for _, ev := range events {
		text := textnorm.Fold(ev.Content + "\n" + ev.Message().Conversation)
		if containsAllTerms(text, terms) {
			kept = append(kept, ev)
		}
	}
	return kept
}

// containsAllTerms expects text already folded.
func containsAllTerms(text string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(text, textnorm.Fold(term)) {
			return false
		}
	}
	return true
}
