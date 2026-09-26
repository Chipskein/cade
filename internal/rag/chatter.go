package rag

import (
	"strings"
	"unicode"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/textnorm"
)

// chatterWords are greetings, acknowledgements and function words: a
// message made only of them ("ok", "valeu", "bom dia", "pode ser") answers
// nothing. Measured by the retrieval suite: in "o que o Marcos me pediu?"
// such messages outranked the actual request among Marcos's messages.
var chatterWords = wordSet(`
oi oie ola opa aoba eai e ai bom boa dia dias tarde noite
ok okay blz beleza valeu vlw obrigado obrigada brigado show top massa certo entendi isso sim nao claro
perfeito otimo joia fechado combinado tranquilo ah hm hmm uai kk kkk kkkk rs haha
pode ser acho que eu tu vc voce aqui ali la ta to ja vou ver tudo bem faz sentido deu tambem tb
o a os as um uma de do da dos das em no na nos nas me te se com pra pro para por
thanks thx thank you yes no hi hello good morning sure got it fine cool great`)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// isChatter reports whether ev is a message with no content word, number
// or link. Only Teams messages qualify: a commit or page is never chatter.
func isChatter(ev event.Event) bool {
	if ev.Source != event.SourceTeams {
		return false
	}
	for _, word := range messageWords(ev) {
		if !chatterWords[word] {
			return false
		}
	}
	return true
}

// messageWords splits the message text (the headline without "Sender: ")
// into folded words; digits stay inside words, so "15h" is content.
func messageWords(ev event.Event) []string {
	text := strings.TrimPrefix(ev.Headline(), ev.Metadata["sender"]+":")
	return strings.FieldsFunc(textnorm.Fold(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// chatterHeadroom is how many more nearest neighbours to fetch so that,
// once chatter is dropped, top_k informative events usually remain.
const chatterHeadroom = 3

// withoutChatter drops chatter from ranked evidence, keeping the order.
func withoutChatter(hits []storage.ScoredEvent) []storage.ScoredEvent {
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		if !isChatter(hit.Event) {
			kept = append(kept, hit)
		}
	}
	return kept
}
