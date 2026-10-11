package idbsource

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/firefoxstorage"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/snappy"
	_ "github.com/mattn/go-sqlite3"
)

// The reviewed ChatGPT drafts schema, read by the real Firefox
// localStorage reader from a synthetic ls directory with the shape of
// Floorp's: no code of ChatGPT's own, only the schema and the configured
// origin.
const (
	chatGPTDraftsSchemaPath = "../../../testdata/idb-schemas/chatgpt-drafts.json"
	chatGPTOrigin           = "https://chatgpt.com"
	chatGPTDrafts           = `{"drafts":[
	{"id":"6abaf266-1e54-83e8-8f20-c9d674667482","content":"  mostrar a imagem do diagrama  ","doc":{"type":"doc"},"timestamp":1790636682621},
	{"id":"6abf95d1-8d0c-83e9-bc03-6cb6351864cc","content":"","timestamp":1790941431821}
],"pendingDrafts":[],"userId":"user-x"}`
)

func writeChatGPTLocalStorage(t *testing.T) string {
	t.Helper()
	location := filepath.Join(t.TempDir(), "https+++chatgpt.com", "ls")
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	db, err := sql.Open("sqlite3", filepath.Join(location, "data.sqlite"))
	testcheck.NoError(t, err)
	defer db.Close()
	for _, statement := range []string{
		"CREATE TABLE database (origin TEXT NOT NULL)",
		"CREATE TABLE data (key TEXT PRIMARY KEY, conversion_type INTEGER NOT NULL, compression_type INTEGER NOT NULL, value BLOB NOT NULL)",
	} {
		_, err = db.Exec(statement)
		testcheck.NoError(t, err)
	}
	_, err = db.Exec("INSERT INTO database VALUES (?)", chatGPTOrigin)
	testcheck.NoError(t, err)
	_, err = db.Exec("INSERT INTO data VALUES ('oai/apps/conversationDrafts', 1, 1, ?), ('client-correlated-secret', 1, 0, 'token')", snappy.Encode(nil, []byte(chatGPTDrafts)))
	testcheck.NoError(t, err)
	return location
}

func collectChatGPTDrafts(t *testing.T) []event.Event {
	t.Helper()
	scope, err := sitestorage.NewScope([]string{chatGPTOrigin})
	testcheck.NoError(t, err)
	open := func(path string) (*sql.DB, error) { return sql.Open("sqlite3", path) }
	readers := webstore.Readers{firefoxstorage.NewLocalStorageReader(scope, open)}
	collector, err := NewCollector(readers, writeChatGPTLocalStorage(t), loadSchema(t, chatGPTDraftsSchemaPath))
	testcheck.NoError(t, err)
	var events []event.Event
	err = collector.CollectEvents(context.Background(), func(ev event.Event) error {
		events = append(events, ev)
		return nil
	})
	testcheck.NoError(t, err)
	return events
}

func TestChatGPTDraftsSchemaKeepsDraftsWithText(t *testing.T) {
	events := collectChatGPTDrafts(t)
	if len(events) != 1 {
		t.Fatalf("expected the one draft with text, got %d: %v", len(events), events)
	}
}

func TestChatGPTDraftsSchemaMapsTheDraft(t *testing.T) {
	ev := collectChatGPTDrafts(t)[0]
	message := ev.Message()
	if message.Text != "mostrar a imagem do diagrama" || message.Sender != "eu" || message.Conversation != "Rascunhos do ChatGPT" {
		t.Fatalf("unexpected message %+v", message)
	}
	if want := time.UnixMilli(1790636682621); !ev.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %v; want %v", ev.Timestamp, want)
	}
	if ev.UID != event.StableID("chatgpt-drafts", "6abaf266-1e54-83e8-8f20-c9d674667482", "6abaf266-1e54-83e8-8f20-c9d674667482") {
		t.Fatal("expected the UID keyed on the conversation id")
	}
}
