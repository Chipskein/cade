package msgstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var sentAt = time.Date(2026, 9, 25, 15, 30, 0, 0, time.UTC)

// syntheticSchemaPath holds the part of a real msgstore.db that Read uses.
const syntheticSchemaPath = "../../testdata/whatsapp-msgstore/schema.sql"

func syntheticSchema(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(syntheticSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(schema)
}

// syntheticRows: Ana's chat (her number maps to a LID) and a group where
// Bia, without a LID, writes; plus rows Read must leave out.
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
			(102, 11, 0, 'CCCC', 4, ` + millisAfter(2) + `, 0, 'bom dia'),
			(103, 11, 0, 'DDDD', NULL, ` + millisAfter(3) + `, 7, 'Ana mudou o nome do grupo'),
			(104, 10, 0, 'EEEE', NULL, ` + millisAfter(4) + `, 15, NULL),
			(105, 10, 1, 'FFFF', NULL, ` + millisAfter(5) + `, 90, NULL),
			(106, 10, 0, 'GGGG', NULL, ` + millisAfter(6) + `, 32, 'tipo novo com texto')`,
		`INSERT INTO message_media VALUES (101, 'Media/WhatsApp Images/IMG-1.jpg', NULL)`,
	}
}

func millisAfter(seconds int64) string {
	return strconv.FormatInt(sentAt.UnixMilli()+seconds*1000, 10)
}

func buildDatabase(t *testing.T, statements ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "msgstore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	return db
}

func readAll(t *testing.T, db *sql.DB) (map[string]Message, error) {
	t.Helper()
	byKey := map[string]Message{}
	err := Read(context.Background(), db, "msgstore.db", func(message Message) error {
		byKey[message.KeyID] = message
		return nil
	})
	return byKey, err
}

func syntheticMessages(t *testing.T) map[string]Message {
	t.Helper()
	messages, err := readAll(t, buildDatabase(t, append([]string{syntheticSchema(t)}, syntheticRows()...)...))
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func TestReadKeepsMessagesAndDropsNoticesCallsAndDeletions(t *testing.T) {
	messages := syntheticMessages(t)
	for _, left := range []string{"DDDD", "EEEE", "FFFF"} {
		if _, found := messages[left]; found {
			t.Errorf("message %s (system, deleted or call) must be left out", left)
		}
	}
	if len(messages) != 4 {
		t.Fatalf("expected AAAA, BBBB, CCCC and the unknown type with text, got %v", messages)
	}
}

func TestReadMapsTheChatToItsLID(t *testing.T) {
	received := syntheticMessages(t)["AAAA"]
	want := Message{KeyID: "AAAA", Type: TypeText, SentAt: sentAt, Text: "oi",
		ChatJID: "5500000000001@s.whatsapp.net", ChatLID: "900000000000001@lid"}
	if received != want {
		t.Fatalf("got %+v, want %+v", received, want)
	}
}

func TestReadKeepsGroupSenderAndSubject(t *testing.T) {
	group := syntheticMessages(t)["CCCC"]
	if group.Subject != "Família" || group.SenderJID != "5500000000002@s.whatsapp.net" || group.SenderLID != "" || group.ChatLID != "" {
		t.Fatalf("expected the group subject and Bia's JID without LID, got %+v", group)
	}
}

func TestReadKeepsMediaWithoutCaptionAndItsFileName(t *testing.T) {
	photo := syntheticMessages(t)["BBBB"]
	if !photo.FromMe || photo.Type != TypeImage || photo.Text != "" || photo.MediaName != "IMG-1.jpg" {
		t.Fatalf("expected my photo named IMG-1.jpg, got %+v", photo)
	}
}

func TestReadRejectsAFileThatIsNotAMsgstore(t *testing.T) {
	_, err := readAll(t, buildDatabase(t, `CREATE TABLE message (_id INTEGER)`))
	if err == nil || !strings.Contains(err.Error(), "decrypted WhatsApp msgstore.db") {
		t.Fatalf("expected an error naming the expected file, got %v", err)
	}
}

func TestIsMessageKeepsUnknownTypesOnlyWithText(t *testing.T) {
	if MessageType(32).IsMessage("") || !MessageType(32).IsMessage("texto") || TypeSystem.IsMessage("texto") || !TypeSticker.IsMessage("") {
		t.Fatal("unknown types need text; system notices never count; stickers always do")
	}
}
