package idbsource

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// The reviewed ChatGPT schema (#78), on synthetic records with the shape
// seen in Chrome's ConversationsDatabase: a message holds only its id and
// text; the conversation id, title and time live on the record holding it.
const (
	chatGPTSchemaPath = "../../../testdata/idb-schemas/chatgpt.json"
	chatGPTDir        = "/home/ana/.config/google-chrome/Default/IndexedDB/https_chatgpt.com_0.indexeddb.leveldb"
	chatGPTUpdateTime = 1727280000.5
)

func chatGPTConversation(id, title string, messages ...*v8value.Value) indexeddb.Record {
	list := &v8value.Value{Kind: v8value.KindArray, Items: messages}
	value := obj("id", str(id), "title", str(title), "updateTime", num(chatGPTUpdateTime), "isArchived", &v8value.Value{Kind: v8value.KindBool}, "messages", list)
	return indexeddb.Record{Namespace: "ConversationsDatabase", Container: "conversations", Value: value}
}

func chatGPTBase() []indexeddb.Record {
	return []indexeddb.Record{
		chatGPTConversation("6abaf266-0000-0000-0000-000000000001", " Relatório da Acme ",
			obj("id", str("m-1"), "text", str("resuma o relatório da Acme")),
			obj("id", str("m-2"), "text", str("  ")),
			obj("id", str("m-3"), "text", str("O relatório cita três prazos."))),
		chatGPTConversation("6abaf266-0000-0000-0000-000000000002", "Viagem", obj("id", str("m-4"), "text", str("roteiro para Orion"))),
		{Namespace: "webrtc-cert-db", Container: "certs", Value: obj("id", str("cert"))},
	}
}

func collectChatGPT(t *testing.T) map[string]event.Event {
	t.Helper()
	collector, err := NewCollector(indexedDBReaders(chatGPTBase(), nil), chatGPTDir, loadSchema(t, chatGPTSchemaPath))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]event.Event{}
	err = collector.CollectEvents(context.Background(), func(ev event.Event) error {
		byID[ev.Message().MessageID] = ev
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return byID
}

func TestChatGPTSchemaKeepsMessagesWithText(t *testing.T) {
	events := collectChatGPT(t)
	if len(events) != 3 {
		t.Fatalf("expected m-1, m-3 and m-4, got %d: %v", len(events), events)
	}
	if _, blank := events["m-2"]; blank {
		t.Fatal("a message with blank text must be left out")
	}
}

func TestChatGPTSchemaTakesTimeAndTitleFromTheConversation(t *testing.T) {
	ev := collectChatGPT(t)["m-3"]
	message := ev.Message()
	if message.ConversationID != "6abaf266-0000-0000-0000-000000000001" || message.Conversation != "Relatório da Acme" || message.Text != "O relatório cita três prazos." {
		t.Fatalf("expected the conversation's id and trimmed title, got %+v", message)
	}
	if !ev.Timestamp.Equal(time.UnixMilli(1727280000500)) {
		t.Fatalf("timestamp %v, want the conversation's updateTime in seconds", ev.Timestamp)
	}
	if ev.UID != event.StableID("chatgpt", "6abaf266-0000-0000-0000-000000000001", "m-3") {
		t.Fatal("expected the UID keyed on the conversation and the message id")
	}
}
