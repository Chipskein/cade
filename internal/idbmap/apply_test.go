package idbmap

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// chainSchema reads a Teams-like store: one record per conversation, its
// messages in a map keyed by id.
const chainSchema = `{
  "version": 1, "target": "message/1", "name": "chat-app", "source": "chat-app", "revision": 1,
  "records": {"database_prefix": "app:chains", "store": "chains", "each": "$.messageMap.<id>"},
  "require": [{"path": "$.messageType", "in": ["Text", "RichText/Html"]}, {"path": "$.deletionInfo", "kind_not": "object"}],
  "fields": {
    "message_id": {"paths": ["$.id"]},
    "conversation_id": {"paths": ["$.conversationId"]},
    "sender": {"paths": ["$.imDisplayName"],
      "lookup": {"database_prefix": "app:profiles", "store": "profiles", "key_path": "$.creator", "match": "$.mri", "value": "$.displayName"},
      "default": "desconhecido"},
    "sender_id": {"paths": ["$.creator"]},
    "text": {"paths": ["$.content"], "transform": "html_text"},
    "sent_at": {"paths": ["$.originalArrivalTime", "$.clientArrivalTime"], "transform": "unix_ms"},
    "sent_by_me": {"paths": ["$.isSentByCurrentUser"]},
    "revision": {"paths": ["$.version"]}
  }
}`

func mustMapper(t *testing.T, raw string) Mapper {
	t.Helper()
	schema, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	mapper, err := NewMapper(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return mapper
}

func record(database, store string, value *v8value.Value) indexeddb.Record {
	return indexeddb.Record{Database: database, Store: store, Value: value}
}

func chainMessage(id, messageType, content string, pairs ...any) *v8value.Value {
	base := []any{"id", str(id), "conversationId", str("19:abc@thread.v2"), "messageType", str(messageType),
		"content", str(content), "originalArrivalTime", num(1727280000000), "creator", str("8:orgid:ana")}
	return obj(append(base, pairs...)...)
}

func chainRecords() []indexeddb.Record {
	messages := obj(
		"1", chainMessage("1", "RichText/Html", "<p>Olá <b>time</b></p>", "imDisplayName", str("Ana Souza"), "version", num(1727280000001)),
		"2", chainMessage("2", "Text", "sem nome", "isSentByCurrentUser", &v8value.Value{Kind: v8value.KindBool, Bool: true}),
		"3", chainMessage("3", "ThreadActivity/AddMember", "<addmember/>"),
		"4", chainMessage("4", "Text", "apagada", "deletionInfo", obj()),
	)
	return []indexeddb.Record{
		record("app:chains:v1", "chains", obj("id", str("19:abc@thread.v2"), "messageMap", messages)),
		record("app:profiles:v1", "profiles", obj("mri", str("8:orgid:ana"), "displayName", str(" Ana (perfil) "))),
		record("other", "chains", obj("messageMap", obj("9", chainMessage("9", "Text", "outro banco")))),
	}
}

func eventsByID(events []event.Event) map[string]event.Message {
	byID := map[string]event.Message{}
	for _, mapped := range events {
		byID[mapped.Message().MessageID] = mapped.Message()
	}
	return byID
}

func TestApplyMapsKeptItemsOnly(t *testing.T) {
	events, tally, err := mustMapper(t, chainSchema).Apply(chainRecords(), "origin")
	if err != nil || len(events) != 2 {
		t.Fatalf("expected messages 1 and 2, got %d events (err %v)", len(events), err)
	}
	if tally != (Tally{Records: 3, StoreRecords: 1, Items: 4, Kept: 2, Mapped: 2}) {
		t.Fatalf("unexpected tally %+v", tally)
	}
}

func TestApplyFillsTheMessage(t *testing.T) {
	events, _, _ := mustMapper(t, chainSchema).Apply(chainRecords(), "origin")
	first := eventsByID(events)["1"]
	want := event.Message{MessageID: "1", ConversationID: "19:abc@thread.v2", Kind: event.KindOther, Sender: "Ana Souza",
		SenderMRI: "8:orgid:ana", Text: "Olá time", Revision: "1727280000001", Origin: "origin"}
	if first != want {
		t.Fatalf("got %+v, want %+v", first, want)
	}
	if !events[0].Timestamp.Equal(time.UnixMilli(1727280000000)) || events[0].Source != "chat-app" {
		t.Fatalf("unexpected timestamp %v / source %q", events[0].Timestamp, events[0].Source)
	}
}

func TestApplyFallsBackToLookupThenDefault(t *testing.T) {
	events, _, _ := mustMapper(t, chainSchema).Apply(chainRecords(), "origin")
	if second := eventsByID(events)["2"]; second.Sender != "Ana (perfil)" || !second.SentByMe {
		t.Fatalf("expected the trimmed profile name and sent by me, got %+v", second)
	}
	records := chainRecords()[:1]
	events, _, _ = mustMapper(t, chainSchema).Apply(records, "origin")
	if second := eventsByID(events)["2"]; second.Sender != "desconhecido" {
		t.Fatalf("expected the default sender without profiles, got %q", second.Sender)
	}
}

// The UID is built like the Teams collector's: same source, conversation
// and message id give the same event, from any origin.
func TestApplyKeysEventsOnSourceConversationAndID(t *testing.T) {
	first, _, _ := mustMapper(t, chainSchema).Apply(chainRecords(), "a")
	second, _, _ := mustMapper(t, chainSchema).Apply(chainRecords(), "b")
	if first[0].UID != second[0].UID || first[0].UID != event.StableID("chat-app", "19:abc@thread.v2", first[0].Message().MessageID) {
		t.Fatal("expected the UID to depend on source, conversation and message id only")
	}
}

func TestApplySkipsItemsWithoutIdentityOrTime(t *testing.T) {
	noTime := obj("id", str("5"), "conversationId", str("c"), "messageType", str("Text"), "content", str("x"))
	records := []indexeddb.Record{record("app:chains", "chains", obj("messageMap", obj("5", noTime)))}
	_, tally, _ := mustMapper(t, chainSchema).Apply(records, "o")
	if tally.Kept != 1 || tally.Mapped != 0 {
		t.Fatalf("expected the item kept but not mapped, got %+v", tally)
	}
}

func TestApplyMapsMetadataOnlyRecordsWithoutText(t *testing.T) {
	message := obj("id", str("false_55@c.us_A"), "from", str("55@c.us"), "t", num(1727280000), "type", str("chat"),
		"msgRowOpaqueData", obj("_keyId", num(1)), "author", obj("_serialized", str("55@c.us")))
	records := []indexeddb.Record{
		record("model-storage", "message", message),
		record("model-storage", "contact", obj("id", str("55@c.us"), "name", str("Ana Souza"))),
	}
	events, _, err := mustMapper(t, metadataOnlySchema).Apply(records, "o")
	if err != nil || len(events) != 1 || events[0].Message().Text != "" || !events[0].Timestamp.Equal(time.Unix(1727280000, 0)) {
		t.Fatalf("expected one event with no text, got %+v (err %v)", events, err)
	}
	if sender := events[0].Message().Sender; sender != "55@c.us" {
		t.Fatalf("expected the author path before the lookup, got %q", sender)
	}
}

func TestTallyCheckReportsFormatChanges(t *testing.T) {
	schema := mustMapper(t, chainSchema).schema
	if err := (Tally{}).Check(schema); err != nil {
		t.Errorf("empty database: %v", err)
	}
	if err := (Tally{Records: 5}).Check(schema); err == nil || !strings.Contains(err.Error(), `store "chains"`) {
		t.Errorf("store gone: expected an error naming the store, got %v", err)
	}
	if err := (Tally{Records: 5, StoreRecords: 1, Items: 3, Kept: 3}).Check(schema); err == nil || !strings.Contains(err.Error(), "none of 3 items") {
		t.Errorf("fields gone: expected an error counting the items, got %v", err)
	}
	if err := (Tally{Records: 5, StoreRecords: 1, Items: 3, Kept: 0}).Check(schema); err != nil {
		t.Errorf("everything filtered out is not a format change: %v", err)
	}
}

func TestNewMapperRejectsInvalidSchemas(t *testing.T) {
	if _, err := NewMapper(Schema{}); err == nil {
		t.Fatal("expected an error for an empty schema")
	}
}
