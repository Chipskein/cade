package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
)

// A WhatsApp photo is the same picture as the file the phone copied to
// "WhatsApp Images", which the vision model described. The message only
// names the file, so the store links it to that file's Image, by name,
// whichever of the two is stored first (#24). The description stays the
// Image's, once (#22): forgetting the file keeps it while a message still
// shows the picture, and forgetting both deletes it.

// filePathKey is event.File's path in the metadata.
const filePathKey = "path"

// attachedImageMatch pairs a message (alias msg) with an image file (alias
// file) whose path ends in the message's attachment name.
var attachedImageMatch = fmt.Sprintf(`file.source = %s AND msg.source <> %s
	AND substr(%s, -length(%s) - 1) = '/' || %s`,
	literal(string(event.SourceFile)), literal(string(event.SourceFile)),
	jsonField("file.metadata", filePathKey), jsonField("msg.metadata", event.AttachmentKey), jsonField("msg.metadata", event.AttachmentKey))

// attachmentLinksQuery links every matching message to the Image its file
// shows; %s narrows it to one side's event.
var attachmentLinksQuery = `INSERT OR IGNORE INTO event_entities (event_id, entity_id, role)
	SELECT msg.id, shown.entity_id, ` + literal(string(entity.RoleShows)) + `
	FROM events AS msg JOIN events AS file ON ` + attachedImageMatch + `
	JOIN event_entities AS shown ON shown.event_id = file.id AND shown.role = ` + literal(string(entity.RoleShows)) + `
	JOIN entities ON entities.id = shown.entity_id AND entities.type = ` + literal(string(entity.TypeImage)) + `
	WHERE %s`

// linkAttachedImages links the stored event eventID, a message naming an
// attachment or an image file, to its counterparts already stored.
func linkAttachedImages(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	side := "file.id = ?"
	if ev.Attachment() != "" {
		side = "msg.id = ?"
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(attachmentLinksQuery, side), eventID); err != nil {
		return fmt.Errorf("link attached images of event %q: %w", ev.UID, err)
	}
	return nil
}

// linkStoredAttachments (schema version 19) links the messages stored
// before, in one statement.
func linkStoredAttachments(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(attachmentLinksQuery, "1")); err != nil {
		return fmt.Errorf("link stored attachments to their images: %w", err)
	}
	return nil
}
