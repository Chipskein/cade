// Package msgstore reads the messages of the WhatsApp Android client's
// database (msgstore.db), already decrypted by the user (#77). Message ids
// (key_id) are the ones WhatsApp Web uses, which is what lets a message
// indexed from the Web by metadata alone get its text from here.
package msgstore

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"
)

// Message is one row of the message table with its chat and sender.
// JIDs are as Android writes them ("5511…@s.whatsapp.net", "…@g.us");
// the LIDs are the ids WhatsApp Web uses for the same people, empty when
// the database maps none.
type Message struct {
	KeyID     string
	FromMe    bool
	Type      MessageType
	SentAt    time.Time
	Text      string
	ChatJID   string
	ChatLID   string
	Subject   string
	SenderJID string
	SenderLID string
	// MediaName is the attachment's file name, without its folder.
	MediaName string
}

// Read emits each message of db, oldest first, leaving out system
// notices, calls and deleted messages. path names the file in errors.
//
//	err := msgstore.Read(ctx, db, "msgstore.db", func(m msgstore.Message) error { … })
func Read(ctx context.Context, db *sql.DB, path string, each func(Message) error) error {
	if err := checkTables(ctx, db, path); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, messagesQuery)
	if err != nil {
		return fmt.Errorf("query messages in %q: %w", path, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := emitRow(rows, each); err != nil {
			return fmt.Errorf("read message in %q: %w", path, err)
		}
	}
	return rows.Err()
}

func emitRow(rows *sql.Rows, each func(Message) error) error {
	message, err := scanMessage(rows)
	if err != nil || !message.Type.IsMessage(message.Text) {
		return err
	}
	return each(message)
}

func scanMessage(rows *sql.Rows) (Message, error) {
	var message Message
	var sentAtMillis int64
	err := rows.Scan(&message.KeyID, &message.FromMe, &message.Type, &sentAtMillis, &message.Text,
		&message.ChatJID, &message.ChatLID, &message.Subject, &message.SenderJID, &message.SenderLID, &message.MediaName)
	message.SentAt = time.UnixMilli(sentAtMillis).UTC()
	if message.MediaName != "" {
		message.MediaName = filepath.Base(message.MediaName)
	}
	return message, err
}

// checkTables rejects a file that is not a decrypted msgstore.db, such as
// the .crypt15 backup itself, naming what was expected.
func checkTables(ctx context.Context, db *sql.DB, path string) error {
	for _, table := range requiredTables {
		var exists bool
		if err := db.QueryRowContext(ctx, tableExistsQuery, table).Scan(&exists); err != nil {
			return fmt.Errorf("read %q as SQLite: %w; expected a decrypted WhatsApp msgstore.db (decrypt the .crypt15 first)", path, err)
		}
		if !exists {
			return fmt.Errorf("%q has no %q table; expected a decrypted WhatsApp msgstore.db with tables %v", path, table, requiredTables)
		}
	}
	return nil
}
