package idbdiscovery

import (
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/llm"
)

const catalogNotation = `Cada store aparece como "S<n> — banco · store · registros", seguido dos caminhos dos valores:
- "$" é o registro (ou o item); ".campo" é uma propriedade; "[]" os itens de uma lista; "{}" os valores de um Map; "<>" os membros de um Set; ".<id>" e ".<text>" são chaves que são dados (ids, textos), como num mapa de mensagens indexado pelo id.
- Depois do caminho vêm os tipos e até duas amostras. As amostras são mascaradas: <email>, <guid> e <n> escondem ids; "<n:13>" é um número de 13 dígitos (milissegundos desde 1970), "<n:10>" um de 10 (segundos); "<date>" é uma data do JavaScript.`

const storeInstructions = `Você mapeia o IndexedDB de um aplicativo para mensagens (conversas, chats, e-mails, posts). Escolha o store que guarda as mensagens individuais, cada uma com autor, hora e texto ou id.
- store: o rótulo S<n> desse store.
- each: quando cada registro guarda várias mensagens (um mapa ou lista delas), o caminho cujos itens são as mensagens; null quando cada registro já é uma mensagem.
` + catalogNotation

const fieldsInstructions = `Você preenche, para as mensagens do store escolhido, de onde vem cada campo. Os caminhos partem do item (a mensagem): "$" é a própria mensagem. Use só caminhos listados; em listas, o primeiro caminho com valor vence.
- message_id: id único da mensagem. conversation_id: id da conversa, chat ou canal a que ela pertence.
- sent_at e time_format: a hora de envio; "unix_ms" para "<n:13>", "unix_s" para "<n:10>", "iso8601" para texto como "2026-09-25T15:00:00Z", "date" para "<date>".
- sender: o nome legível de quem enviou; sender_id: o id de quem enviou. conversation: o nome ou título da conversa, [] se não houver.
- text e text_format: o corpo da mensagem; "html_text" quando as amostras têm tags HTML, senão "plain". [] quando o corpo não aparece (por exemplo, cifrado num campo binário).
- sent_by_me: um booleano verdadeiro quando o próprio usuário enviou; [] se não houver.
- keep: quando o store mistura mensagens com eventos de sistema (entradas no grupo, chamadas, notificações), o campo de tipo e os valores das mensagens escritas por pessoas, copiados das amostras; senão null.
- sender_lookup: quando o remetente é só um id e outro store tem o nome: o store, key (caminho do id no item), match (caminho do id no outro store) e value (caminho do nome no outro store); senão null.
` + catalogNotation

// The examples use an invented application, so no real app's layout is
// taught as the answer.
const (
	exampleStoreCatalog = `S1 — banco "notas-app" · store "drafts" · 12 registros
  $                           object
  $.body                      string  "lembrar do relatório"
  $.updated                   number  "<n:13>"
S2 — banco "chat-app:<guid>" · store "threads" · 40 registros
  $                           object
  $.id                        string  "<email>"
  $.posts                     array
  $.posts[]                   object
  $.posts[].author            string  "u-81"
  $.posts[].at                string  "2026-09-25T15:00:00Z"
  $.posts[].html              string  "<p>bom dia <b>time</b></p>"
  $.posts[].kind              string  "post", "join"
  $.posts[].postId            string  "p-<n>"
S3 — banco "chat-app:<guid>" · store "people" · 9 registros
  $                           object
  $.uid                       string  "u-81"
  $.fullName                  string  "Carla Dias"`
	exampleStoreReply     = `{"store": "S2", "each": "$.posts[]"}`
	exampleFieldsQuestion = `Mensagens (itens de S2 em $.posts[]):
S2 — banco "chat-app:<guid>" · store "threads" · 40 registros
  $.author                    string  "u-81"
  $.at                        string  "2026-09-25T15:00:00Z"
  $.html                      string  "<p>bom dia <b>time</b></p>"
  $.kind                      string  "post", "join"
  $.postId                    string  "p-<n>"
  $.thread                    string  "<email>"

Outros stores:
S1 — banco "notas-app" · store "drafts" · 12 registros
  $.body                      string  "lembrar do relatório"
S3 — banco "chat-app:<guid>" · store "people" · 9 registros
  $.uid                       string  "u-81"
  $.fullName                  string  "Carla Dias"`
	exampleFieldsReply = `{"message_id": ["$.postId"], "conversation_id": ["$.thread"], "sent_at": ["$.at"], "time_format": "iso8601", "sender": [], "sender_id": ["$.author"], "conversation": [], "text": ["$.html"], "text_format": "html_text", "sent_by_me": [], "keep": {"path": "$.kind", "in": ["post"]}, "sender_lookup": {"store": "S3", "key": "$.author", "match": "$.uid", "value": "$.fullName"}}`
)

// storeMessages asks for the store; note, when not empty, tells the model
// about the schema being regenerated.
func storeMessages(catalog Catalog, note string) []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: llm.RoleSystem, Content: storeInstructions},
		{Role: llm.RoleUser, Content: exampleStoreCatalog},
		{Role: llm.RoleAssistant, Content: exampleStoreReply},
		{Role: llm.RoleUser, Content: withNote(renderCatalog(catalog.Stores), note)},
	}
}

func withNote(question, note string) string {
	if note == "" {
		return question
	}
	return question + "\n\n" + note
}

// fieldsMessages shows the chosen store's item paths first, then the other
// stores for the sender lookup.
func fieldsMessages(catalog Catalog, store StoreView, each idbmap.Path, note string) []llm.ChatMessage {
	chosen := store
	chosen.Paths = store.Under(each)
	var others []StoreView
	for _, other := range catalog.Stores {
		if other.Label != store.Label {
			others = append(others, other)
		}
	}
	question := "Mensagens (itens de " + itemLabel(store, each) + "):\n" + renderCatalog([]StoreView{chosen}) + "\n\nOutros stores:\n" + renderCatalog(others)
	return []llm.ChatMessage{
		{Role: llm.RoleSystem, Content: fieldsInstructions},
		{Role: llm.RoleUser, Content: exampleFieldsQuestion},
		{Role: llm.RoleAssistant, Content: exampleFieldsReply},
		{Role: llm.RoleUser, Content: withNote(question, note)},
	}
}

func itemLabel(store StoreView, each idbmap.Path) string {
	if each == "" {
		return store.Label
	}
	return store.Label + " em " + string(each)
}

func renderCatalog(stores []StoreView) string {
	var lines []string
	for _, store := range stores {
		lines = append(lines, fmt.Sprintf("%s — banco %q · store %q · %d registros", store.Label, store.MaskedDatabase, store.Store, store.Records))
		for _, path := range store.Paths {
			lines = append(lines, renderPath(path))
		}
	}
	return strings.Join(lines, "\n")
}

func renderPath(path PathView) string {
	kinds := make([]string, len(path.Kinds))
	for i, kind := range path.Kinds {
		kinds[i] = kind.String()
	}
	samples := make([]string, len(path.Samples))
	for i, sample := range path.Samples {
		samples[i] = fmt.Sprintf("%q", sample)
	}
	return strings.TrimRight(fmt.Sprintf("  %-28s%-8s%s", path.Path, strings.Join(kinds, "|"), strings.Join(samples, ", ")), " ")
}
