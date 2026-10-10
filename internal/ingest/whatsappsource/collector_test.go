package whatsappsource

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/testfakes"
)

const syntheticSchemaPath = "../../../testdata/whatsapp-msgstore/schema.sql"

var sentAt = time.Date(2026, 9, 25, 15, 30, 0, 0, time.UTC)

func openSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

// syntheticRows: Ana's chat (her number maps to a LID), where she wrote
// "oi" and I sent a photo, and the Família group where Carla, without a
// LID, wrote "bom dia".
func syntheticRows() []string {
	return []string{
		`INSERT INTO jid VALUES (1, '5500000000001', 's.whatsapp.net', '5500000000001@s.whatsapp.net'),
			(2, '900000000000001', 'lid', '900000000000001@lid'),
			(3, '120000000000000001', 'g.us', '120000000000000001@g.us'),
			(4, '5500000000002', 's.whatsapp.net', '5500000000002@s.whatsapp.net')`,
		`INSERT INTO jid_map VALUES (2, 1, 0)`,
		`INSERT INTO chat VALUES (10, 1, NULL), (11, 3, 'Família')`,
		`INSERT INTO message VALUES
			(100, 10, 0, 'AAAA', NULL, ` + millisAfter(0) + `, 0, 'oi'),
			(101, 10, 1, 'BBBB', NULL, ` + millisAfter(1) + `, 1, NULL),
			(102, 11, 0, 'CCCC', 4, ` + millisAfter(2) + `, 0, 'bom dia')`,
		`INSERT INTO message_media VALUES (101, 'Media/WhatsApp Images/IMG-1.jpg', NULL)`,
	}
}

func millisAfter(seconds int64) string {
	return strconv.FormatInt(sentAt.UnixMilli()+seconds*1000, 10)
}

func buildMsgstore(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(syntheticSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "msgstore.db")
	db, _ := openSQLite(path)
	defer db.Close()
	for _, statement := range append([]string{string(schema)}, syntheticRows()...) {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	return path
}

// webEvent is Ana's "oi" as the WhatsApp schema stored it: no text.
func webEvent() event.Event {
	message := event.Message{ConversationID: "900000000000001@lid", MessageID: "false_900000000000001@lid_AAAA", Kind: event.KindOther,
		Sender: "Ana", SenderMRI: "900000000000001@lid", Conversation: "", Origin: "idb"}
	metadata := message.Metadata()
	metadata[event.SchemaKey], metadata[event.SchemaRevisionKey] = "whatsapp", "1"
	return event.Event{UID: event.StableID(event.SourceWhatsApp, message.ConversationID, message.MessageID), Timestamp: sentAt,
		Source: event.SourceWhatsApp, Content: message.Content(), Metadata: metadata}
}

func newPipeline(store *testfakes.FakeEventStore) *ingest.Pipeline {
	return ingest.NewPipeline(store, &testfakes.FakeEmbedder{}, "doc: ", slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func eventsByMessageID(store *testfakes.FakeEventStore) map[string]event.Event {
	byID := map[string]event.Event{}
	for _, ev := range store.Events {
		byID[ev.Message().MessageID] = ev
	}
	return byID
}

func TestBackupFillsTheTextOfAMessageStoredByMetadataWithoutDuplicating(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{webEvent()}
	collector := NewCollector(openSQLite, buildMsgstore(t))
	report, err := newPipeline(store).Run(context.Background(), collector, nil)
	if err != nil || report != (ingest.Report{Collected: 3, Updated: 1, Inserted: 2}) || collector.Tally() != (Tally{Paired: 1, New: 2}) {
		t.Fatalf("expected Ana's message completed and two new, got %+v, %+v (err %v)", report, collector.Tally(), err)
	}
	filled := eventsByMessageID(store)["false_900000000000001@lid_AAAA"]
	if filled.UID != webEvent().UID || filled.Message().Text != "oi" || filled.Metadata[event.SchemaKey] != "whatsapp" || !filled.TextFromBackup() {
		t.Fatalf("expected the Web event with the text and its schema marks, got %+v", filled)
	}
}

func TestBackupImportedTwiceChangesNothing(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{webEvent()}
	path := buildMsgstore(t)
	if _, err := newPipeline(store).Run(context.Background(), NewCollector(openSQLite, path), nil); err != nil {
		t.Fatal(err)
	}
	report, err := newPipeline(store).Run(context.Background(), NewCollector(openSQLite, path), nil)
	if err != nil || report != (ingest.Report{Collected: 3, AlreadyStored: 3}) || len(store.Events) != 3 {
		t.Fatalf("expected the second import to store nothing, got %+v with %d events (err %v)", report, len(store.Events), err)
	}
}

func TestMessagesOnlyInTheBackupAreKeyedAsTheWebWould(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	if _, err := newPipeline(store).Run(context.Background(), NewCollector(openSQLite, buildMsgstore(t)), nil); err != nil {
		t.Fatal(err)
	}
	group := eventsByMessageID(store)["false_120000000000000001@g.us_CCCC_5500000000002@c.us"]
	if group.UID != event.StableID(event.SourceWhatsApp, "120000000000000001@g.us", "false_120000000000000001@g.us_CCCC_5500000000002@c.us") {
		t.Fatalf("expected the group message keyed on the chat and the Web id, got %+v", store.Events)
	}
	if group.Message().Conversation != "Família" || group.Message().Sender != unknownSender || group.Message().Text != "bom dia" {
		t.Fatalf("expected the group subject, an unknown sender and the text, got %+v", group.Message())
	}
}

func TestAPhotoWithoutCaptionKeepsItsFileName(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	if _, err := newPipeline(store).Run(context.Background(), NewCollector(openSQLite, buildMsgstore(t)), nil); err != nil {
		t.Fatal(err)
	}
	photo := eventsByMessageID(store)["true_900000000000001@lid_BBBB"]
	if !photo.Message().SentByMe || photo.Message().Text != "" || photo.Metadata[attachmentKey] != "IMG-1.jpg" || photo.TextFromBackup() {
		t.Fatalf("expected my photo, without text, named IMG-1.jpg, got %+v", photo)
	}
}

func TestNamesComeFromTheWebEvents(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	sent := event.Message{ConversationID: "x@lid", MessageID: "true_x@lid_ZZZZ", Sender: "Eu", SentByMe: true}
	store.Events = []event.Event{{UID: "sent", Source: event.SourceWhatsApp, Timestamp: sentAt, Metadata: sent.Metadata()}}
	webAna := webEvent()
	webAna.UID, webAna.Metadata["message_id"] = "another of Ana's", "false_900000000000001@lid_OTHER"
	store.Events = append(store.Events, webAna)
	if _, err := newPipeline(store).Run(context.Background(), NewCollector(openSQLite, buildMsgstore(t)), nil); err != nil {
		t.Fatal(err)
	}
	byID := eventsByMessageID(store)
	if byID["false_900000000000001@lid_AAAA"].Message().Sender != "Ana" || byID["true_900000000000001@lid_BBBB"].Message().Sender != "Eu" {
		t.Fatalf("expected Ana's name from her Web messages and mine from my sent ones, got %+v", store.Events)
	}
}
