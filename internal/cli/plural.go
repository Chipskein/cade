package cli

import "fmt"

// nounForms are a counted phrase's singular and plural in each language;
// the phrase carries the words that agree with the count ("tarefa sua",
// "tarefas suas").
type nounForms struct {
	portugueseOne, portugueseMany string
	englishOne, englishMany       string
}

// count writes n followed by the form of forms that agrees with it in l:
// "1 evento", "0 eventos", "2 events". Both languages use the singular for
// 1 only; Portuguese grammars also allow "0 evento", but "0 eventos" is
// what people write.
func (l Language) count(n int, forms nounForms) string {
	one, many := forms.portugueseOne, forms.portugueseMany
	if l == English {
		one, many = forms.englishOne, forms.englishMany
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Counted phrases shared by several messages.
var (
	eventNoun = nounForms{"evento", "eventos", "event", "events"}
	taskNoun  = nounForms{"tarefa", "tarefas", "task", "tasks"}
)
