package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

var webMessageTime = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// webMessage is a WhatsApp Web message mapped without its text.
func webMessage() event.Event {
	message := event.Message{ConversationID: "c@lid", MessageID: "false_c@lid_AAAA", Sender: "Ana", Kind: event.KindOther}
	return event.Event{UID: "web", Source: event.SourceWhatsApp, Timestamp: webMessageTime, Content: message.Content(), Metadata: message.Metadata()}
}

// withBackupText is ev completed with text from the Android database.
func withBackupText(ev event.Event, text string) event.Event {
	message := ev.Message()
	message.Text = text
	metadata := message.Metadata()
	metadata[event.TextOriginKey] = event.TextOriginBackup
	ev.Content, ev.Metadata = message.Content(), metadata
	return ev
}

// FakePairingCollector emits the stored WhatsApp events with Text, and
// records whether the pipeline gave it the store.
type FakePairingCollector struct {
	Text        string
	ReadsStored bool
}

func (f *FakePairingCollector) CollectEvents(context.Context, EmitFunc) error {
	return nil
}

func (f *FakePairingCollector) CollectPaired(ctx context.Context, stored StoredEventsReader, emit EmitFunc) error {
	f.ReadsStored = true
	found, err := stored.EventsMatching(ctx, storage.EventFilter{Source: event.SourceWhatsApp, From: webMessageTime, To: webMessageTime.Add(time.Second)})
	for _, ev := range found {
		if err := emit(withBackupText(ev, f.Text)); err != nil {
			return err
		}
	}
	return err
}

func TestRunGivesAPairingCollectorTheStoredEvents(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{webMessage()}
	collector := &FakePairingCollector{Text: "oi"}
	report, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), collector, nil)
	if err != nil || !collector.ReadsStored || report != (Report{Collected: 1, Updated: 1}) || store.Events[0].Message().Text != "oi" {
		t.Fatalf("expected the stored message to get its text, got %+v, %+v (err %v)", report, store.Events, err)
	}
}

// The WhatsApp Web schema has no revision: without the rule, the next
// `ingest whatsapp` would replace the filled-in text with nothing.
func TestRunKeepsBackupTextAgainstAReadingWithoutText(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{withBackupText(webMessage(), "oi")}
	report, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), FakeCollector{Events: []event.Event{webMessage()}}, nil)
	if err != nil || report != (Report{Collected: 1, AlreadyStored: 1}) || store.Events[0].Message().Text != "oi" {
		t.Fatalf("expected the backup text kept, got %+v, %q (err %v)", report, store.Events[0].Message().Text, err)
	}
}

func TestReplacesBackupTextOnlyWithText(t *testing.T) {
	stored := withBackupText(webMessage(), "oi")
	if replaces(webMessage(), stored) || !replaces(withBackupText(webMessage(), "oi, editada"), stored) {
		t.Fatal("expected a reading without text to keep the backup's, and an edited backup text to replace it")
	}
}
