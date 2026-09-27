package teamssource

import (
	"context"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
)

// expectedMessage is what one message of a format sample must become.
type expectedMessage struct {
	text         string
	sender       string
	conversation string
	sentByMe     bool
}

// formatSamples are Teams caches in each format seen so far, written by a
// real Chrome from synthetic pages (testdata/chrome-indexeddb-pages, see
// testdata/README.md). A reader change that stops recognizing one fails
// here instead of on a user's cache. A new format gets its own page,
// sample and entry.
var formatSamples = []struct {
	dir      string
	messages []expectedMessage
}{
	{dir: "../../../testdata/teams-formats/2026-09.leveldb", messages: []expectedMessage{
		{text: "O deploy da API ficou para sexta às 15h.", sender: "Ana Souza", conversation: "Ana Souza"},
		{text: "combinado", sender: "Eu", conversation: "Ana Souza", sentByMe: true},
		{text: "Rui, o PROJ-481 já está em produção?", sender: "Carla Dias", conversation: "Acme › Deploy"},
	}},
}

func TestKnownFormatSamplesAreRecognized(t *testing.T) {
	for _, sample := range formatSamples {
		messages, err := collectSample(sample.dir)
		if err != nil {
			t.Fatalf("%s: format not recognized: %v", sample.dir, err)
		}
		if len(messages) != len(sample.messages) {
			t.Errorf("%s: expected %d messages (deleted and system ones skipped), got %d: %+v", sample.dir, len(sample.messages), len(messages), messages)
		}
		for _, want := range sample.messages {
			requireMessage(t, sample.dir, messages, want)
		}
	}
}

func collectSample(dir string) ([]event.Message, error) {
	var messages []event.Message
	err := NewCollector(indexeddb.ReadDirectory, dir).CollectEvents(context.Background(), func(ev event.Event) error {
		messages = append(messages, ev.Message())
		return nil
	})
	return messages, err
}

func requireMessage(t *testing.T, dir string, messages []event.Message, want expectedMessage) {
	t.Helper()
	for _, got := range messages {
		if !strings.Contains(got.Text, want.text) {
			continue
		}
		if got.Sender != want.sender || got.Conversation != want.conversation || got.SentByMe != want.sentByMe {
			t.Errorf("%s: message %q read as %+v, expected %+v", dir, want.text, got, want)
		}
		return
	}
	t.Errorf("%s: message %q missing from %+v", dir, want.text, messages)
}
