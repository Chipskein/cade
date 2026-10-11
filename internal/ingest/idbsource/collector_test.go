package idbsource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// indexedDBReaders read any location as an IndexedDB holding records, or
// fail with err.
func indexedDBReaders(records []webstore.Record, err error) webstore.Readers {
	return webstore.Readers{&testfakes.FakeStoreReader{StoreKind: webstore.KindIndexedDB, Records: records, Err: err}}
}

func str(text string) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindString, Text: text}
}

func num(number float64) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindNumber, Number: number}
}

// obj builds an object; a key given again replaces the earlier value, so
// a test can override one field of a base message.
func obj(pairs ...any) *v8value.Value {
	object := &v8value.Value{Kind: v8value.KindObject}
	for i := 0; i+1 < len(pairs); i += 2 {
		key, value := pairs[i].(string), pairs[i+1].(*v8value.Value)
		if existing := object.Get(key); existing != nil {
			*existing = *value
			continue
		}
		object.Properties = append(object.Properties, v8value.Property{Key: key, Value: value})
	}
	return object
}

const whatsappDir = "/home/ana/.floorp/p/storage/default/https+++web.whatsapp.com/idb"

// chatSchema reads one message per record, by metadata only.
const chatSchema = `{"version": 1, "target": "message/1", "name": "whatsapp", "source": "whatsapp",
  "records": {"database_prefix": "model-storage", "store": "message"},
  "fields": {"message_id": {"paths": ["$.id"]}, "conversation_id": {"paths": ["$.from"]}, "sent_at": {"paths": ["$.t"], "transform": "unix_s"}}}`

func chatSchemaParsed(t *testing.T) idbmap.Schema {
	t.Helper()
	schema, err := idbmap.Parse([]byte(chatSchema))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func whatsappRecords() []webstore.Record {
	message := func(id string) *v8value.Value {
		return obj("id", str(id), "from", str("55@c.us"), "t", num(1727280000), "type", str("chat"))
	}
	return []webstore.Record{
		{Origin: "https+++web.whatsapp.com", Namespace: "model-storage", Container: "message", Value: message("A")},
		{Origin: "https+++web.whatsapp.com", Namespace: "model-storage", Container: "message", Value: message("B")},
	}
}

func newWhatsappCollector(t *testing.T, readers webstore.Readers) *Collector {
	t.Helper()
	collector, err := NewCollector(readers, whatsappDir, chatSchemaParsed(t))
	if err != nil {
		t.Fatal(err)
	}
	return collector
}

func TestCollectEventsEmitsMappedMessagesWithTheirOrigin(t *testing.T) {
	var emitted []event.Event
	err := newWhatsappCollector(t, indexedDBReaders(whatsappRecords(), nil)).CollectEvents(context.Background(), func(ev event.Event) error {
		emitted = append(emitted, ev)
		return nil
	})
	if err != nil || len(emitted) != 2 || emitted[0].Source != "whatsapp" || emitted[0].Message().Origin != "https+++web.whatsapp.com" {
		t.Fatalf("expected two whatsapp events from the origin, got %+v (err %v)", emitted, err)
	}
}

func TestCollectEventsReportsAFormatChange(t *testing.T) {
	records := []webstore.Record{{Namespace: "model-storage", Container: "mensagens", Value: obj("id", str("A"))}}
	err := newWhatsappCollector(t, indexedDBReaders(records, nil)).CollectEvents(context.Background(), func(event.Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "changed its format") || !strings.Contains(err.Error(), "cade schema-check --update whatsapp") {
		t.Fatalf("expected a format change error, got %v", err)
	}
}

func TestCollectEventsStopsOnErrors(t *testing.T) {
	failure := errors.New("disk")
	if err := newWhatsappCollector(t, indexedDBReaders(nil, failure)).CollectEvents(context.Background(), nil); !errors.Is(err, failure) {
		t.Errorf("read error: got %v", err)
	}
	collector := newWhatsappCollector(t, indexedDBReaders(whatsappRecords(), nil))
	if err := collector.CollectEvents(context.Background(), func(event.Event) error { return failure }); !errors.Is(err, failure) {
		t.Errorf("emit error: got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := collector.CollectEvents(ctx, func(event.Event) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: got %v", err)
	}
}

func TestNewCollectorRejectsInvalidSchemas(t *testing.T) {
	schema := chatSchemaParsed(t)
	schema.Records.Container = ""
	if _, err := NewCollector(indexedDBReaders(nil, nil), whatsappDir, schema); err == nil {
		t.Fatal("expected an error for a schema without store")
	}
}

// chatHistorySchema reads a chat kept in one localStorage key.
const chatHistorySchema = `{"version": 2, "target": "message/1", "name": "chat", "source": "chat",
  "records": {"kind": "local_storage", "container": "chat:history", "each": "$.messages[]"},
  "fields": {"message_id": {"paths": ["$.id"]}, "conversation_id": {"paths": ["$.room"]}, "sent_at": {"paths": ["$.at"], "transform": "unix_s"}}}`

// A storage cade gains later (#74, #75) only implements webstore.Reader:
// the collector reads it through the kind the schema names.
func TestCollectEventsReadsTheStorageTheSchemaNames(t *testing.T) {
	schema, err := idbmap.Parse([]byte(chatHistorySchema))
	if err != nil {
		t.Fatal(err)
	}
	history := obj("messages", &v8value.Value{Kind: v8value.KindArray, Items: []*v8value.Value{obj("id", str("m1"), "room", str("geral"), "at", num(1727280000))}})
	local := &testfakes.FakeStoreReader{StoreKind: webstore.KindLocalStorage, Suffix: "/ls",
		Records: []webstore.Record{{Kind: webstore.KindLocalStorage, Origin: "https_chat.example_0", Container: "chat:history", Value: history}}}
	indexedDB := &testfakes.FakeStoreReader{StoreKind: webstore.KindIndexedDB}
	collector, err := NewCollector(webstore.Readers{indexedDB, local}, "/p/https_chat.example_0/ls", schema)
	var emitted []event.Event
	if err == nil {
		err = collector.CollectEvents(context.Background(), func(ev event.Event) error { emitted = append(emitted, ev); return nil })
	}
	if err != nil || len(emitted) != 1 || emitted[0].Message().Origin != "https_chat.example_0" || len(indexedDB.ReadLocations) != 0 {
		t.Fatalf("collect = %+v, %v (IndexedDB reads %v); want the localStorage message only", emitted, err, indexedDB.ReadLocations)
	}
}
