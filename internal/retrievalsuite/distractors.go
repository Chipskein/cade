package retrievalsuite

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// Distractors grow the corpus toward a real history's size: with more
// events, the nearest neighbour of any question gets closer, so a gate
// tuned on a few hundred events lets more unanswerable questions through.
// They mix everyday work topics that answer none of the cases.

var (
	distractorSubjects = strings.Fields(`pedidos relatórios usuários pagamentos estoque notificações faturas auditoria cadastro
		permissões exportação importação agendamento dashboard filtros paginação cache fila webhook integração`)
	distractorActions  = []string{"Ajusta", "Refatora", "Corrige validação de", "Adiciona testes de", "Remove código morto de", "Melhora logs de", "Documenta"}
	distractorMessages = []string{"alguém revisou o PR de %s?", "subi a correção de %s em hml", "a tela de %s está lenta hoje",
		"precisamos alinhar o fluxo de %s", "o cliente perguntou sobre %s", "vou pegar a demanda de %s", "reunião sobre %s às 15h"}
	distractorPages = []string{"Stack Overflow - %s em Go", "Guia de %s - documentação interna", "Como testar %s", "Boas práticas de %s"}
	distractorNames = []string{"Pedro Alves", "Beatriz Nunes", "Rui Costa", "Carla Dias", "Marcos Lima", "Juliana Prado", "Leandro Silva"}
)

// WithDistractors returns the suite with synthetic events added until the
// corpus has total events; the same total always yields the same events.
func (s Suite) WithDistractors(total int) Suite {
	random := rand.New(rand.NewSource(int64(total)))
	s.distractors = nil
	for i := len(s.Events); i < total; i++ {
		s.distractors = append(s.distractors, distractorEvent(random, i, s.Now))
	}
	return s
}

func distractorEvent(random *rand.Rand, i int, now time.Time) event.Event {
	subject := distractorSubjects[random.Intn(len(distractorSubjects))]
	ev := event.Event{UID: fmt.Sprintf("distractor-%d", i), Timestamp: now.Add(-time.Duration(random.Int63n(int64(180 * 24 * time.Hour))))}
	switch i % 3 {
	case 0:
		return distractorCommit(random, ev, subject)
	case 1:
		return distractorMessage(random, ev, subject)
	}
	return distractorVisit(random, ev, subject, i)
}

func distractorCommit(random *rand.Rand, ev event.Event, subject string) event.Event {
	ev.Source, ev.Content = event.SourceGit, distractorActions[random.Intn(len(distractorActions))]+" "+subject
	ev.Metadata = event.Commit{Repository: "/src/api", Hash: fmt.Sprintf("%040x", random.Uint64()), Author: "Eu"}.Metadata()
	return ev
}

func distractorMessage(random *rand.Rand, ev event.Event, subject string) event.Event {
	sender := distractorNames[random.Intn(len(distractorNames))]
	message := event.Message{Sender: sender, Conversation: sender + ", Eu", Kind: event.KindChat,
		Text: fmt.Sprintf(distractorMessages[random.Intn(len(distractorMessages))], subject)}
	ev.Source, ev.Content, ev.Metadata = event.SourceTeams, message.Content(), message.Metadata()
	return ev
}

func distractorVisit(random *rand.Rand, ev event.Event, subject string, i int) event.Event {
	title := fmt.Sprintf(distractorPages[random.Intn(len(distractorPages))], subject)
	url := fmt.Sprintf("https://docs.example.com/%s/%d", subject, i)
	ev.Source, ev.Content, ev.Metadata = event.SourceBrowser, title+"\n"+url, event.Visit{URL: url, Title: title}.Metadata()
	return ev
}
