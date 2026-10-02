package idbsource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// FakeIndexedDBReader returns fixed records, or fails.
type FakeIndexedDBReader struct {
	Records  []indexeddb.Record
	FailWith error
}

func (f FakeIndexedDBReader) Read(string) ([]indexeddb.Record, error) {
	return f.Records, f.FailWith
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

func whatsappRecords() []indexeddb.Record {
	message := func(id string) *v8value.Value {
		return obj("id", str(id), "from", str("55@c.us"), "t", num(1727280000), "type", str("chat"))
	}
	return []indexeddb.Record{
		{Database: "model-storage", Store: "message", Value: message("A")},
		{Database: "model-storage", Store: "message", Value: message("B")},
	}
}

func newWhatsappCollector(t *testing.T, reader FakeIndexedDBReader) *Collector {
	t.Helper()
	collector, err := NewCollector(reader.Read, whatsappDir, chatSchemaParsed(t))
	if err != nil {
		t.Fatal(err)
	}
	return collector
}

func TestCollectEventsEmitsMappedMessagesWithTheirOrigin(t *testing.T) {
	var emitted []event.Event
	err := newWhatsappCollector(t, FakeIndexedDBReader{Records: whatsappRecords()}).CollectEvents(context.Background(), func(ev event.Event) error {
		emitted = append(emitted, ev)
		return nil
	})
	if err != nil || len(emitted) != 2 || emitted[0].Source != "whatsapp" || emitted[0].Message().Origin != "https+++web.whatsapp.com" {
		t.Fatalf("expected two whatsapp events from the origin, got %+v (err %v)", emitted, err)
	}
}

func TestCollectEventsReportsAFormatChange(t *testing.T) {
	records := []indexeddb.Record{{Database: "model-storage", Store: "mensagens", Value: obj("id", str("A"))}}
	err := newWhatsappCollector(t, FakeIndexedDBReader{Records: records}).CollectEvents(context.Background(), func(event.Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "changed its format") {
		t.Fatalf("expected a format change error, got %v", err)
	}
}

func TestCollectEventsStopsOnErrors(t *testing.T) {
	failure := errors.New("disk")
	if err := newWhatsappCollector(t, FakeIndexedDBReader{FailWith: failure}).CollectEvents(context.Background(), nil); !errors.Is(err, failure) {
		t.Errorf("read error: got %v", err)
	}
	collector := newWhatsappCollector(t, FakeIndexedDBReader{Records: whatsappRecords()})
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
	schema.Records.Store = ""
	if _, err := NewCollector(FakeIndexedDBReader{}.Read, whatsappDir, schema); err == nil {
		t.Fatal("expected an error for a schema without store")
	}
}
