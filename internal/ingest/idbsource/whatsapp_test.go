package idbsource

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// The reviewed WhatsApp Web schema, on synthetic records with the shape
// seen in a real base: the body is encrypted, so only metadata maps.
const whatsappSchemaPath = "../../../testdata/idb-schemas/whatsapp.json"

func whatsappMessage(id, messageType string, pairs ...any) indexeddb.Record {
	encrypted := obj("_data", &v8value.Value{Kind: v8value.KindBinary, Bytes: []byte{1, 2}}, "_keyId", num(7))
	base := []any{"id", str(id), "type", str(messageType), "t", num(1727280000), "from", str("5500000000001@c.us"), "msgRowOpaqueData", encrypted}
	return indexeddb.Record{Namespace: "model-storage", Container: "message", Value: obj(append(base, pairs...)...)}
}

func whatsappBase() []indexeddb.Record {
	return []indexeddb.Record{
		whatsappMessage("false_5500000000001@c.us_AAAA", "chat"),
		whatsappMessage("true_5500000000001@c.us_BBBB", "image", "from", str("5500000000009@c.us")),
		whatsappMessage("false_120000000000000001@g.us_CCCC_5500000000002@c.us", "chat",
			"from", str("120000000000000001@g.us"), "author", obj("_serialized", str("5500000000002@c.us"))),
		whatsappMessage("false_120000000000000001@g.us_DDDD", "gp2", "from", str("120000000000000001@g.us")),
		{Namespace: "model-storage", Container: "contact", Value: obj("id", str("5500000000001@c.us"), "pushname", str("Ana"))},
		{Namespace: "model-storage", Container: "contact", Value: obj("id", str("5500000000002@c.us"), "name", str("Carla Dias"), "pushname", str("Carlinha"))},
		{Namespace: "model-storage", Container: "chat", Value: obj("id", str("120000000000000001@g.us"), "name", str("Família"))},
	}
}

func collectWhatsapp(t *testing.T) map[string]event.Event {
	t.Helper()
	collector, err := NewCollector(FakeIndexedDBReader{Records: whatsappBase()}.Read, whatsappDir, loadSchema(t, whatsappSchemaPath))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]event.Event{}
	err = collector.CollectEvents(context.Background(), func(ev event.Event) error {
		byID[strings.Split(ev.Message().MessageID, "_")[2]] = ev
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return byID
}

func TestWhatsappSchemaKeepsMessagesAndDropsSystemEvents(t *testing.T) {
	events := collectWhatsapp(t)
	if len(events) != 3 {
		t.Fatalf("expected the chat, image and group messages, got %d: %v", len(events), events)
	}
	if _, system := events["DDDD"]; system {
		t.Fatal("a gp2 system event must be left out")
	}
}

func TestWhatsappSchemaReadsTheCompositeID(t *testing.T) {
	events := collectWhatsapp(t)
	received, sent := events["AAAA"].Message(), events["BBBB"].Message()
	if received.ConversationID != "5500000000001@c.us" || received.SentByMe || !sent.SentByMe {
		t.Fatalf("unexpected conversation or direction: %+v / %+v", received, sent)
	}
	if received.Sender != "Ana" || received.Text != "" || !events["AAAA"].Timestamp.Equal(time.Unix(1727280000, 0)) {
		t.Fatalf("expected Ana's pushname, no text and the time in seconds, got %+v", received)
	}
	if events["AAAA"].UID != event.StableID("whatsapp", "5500000000001@c.us", "false_5500000000001@c.us_AAAA") {
		t.Fatal("expected the UID keyed on the chat and the whole message id")
	}
}

func TestWhatsappSchemaNamesGroupsAndTheirAuthors(t *testing.T) {
	group := collectWhatsapp(t)["CCCC"].Message()
	if group.ConversationID != "120000000000000001@g.us" || group.Conversation != "Família" || group.Sender != "Carla Dias" || group.SenderMRI != "5500000000002@c.us" {
		t.Fatalf("expected the group name and the author's address book name, got %+v", group)
	}
	if unknown := collectWhatsapp(t)["BBBB"].Message(); unknown.Sender != "desconhecido" {
		t.Fatalf("a sender missing from the contacts is unknown, got %q", unknown.Sender)
	}
}
