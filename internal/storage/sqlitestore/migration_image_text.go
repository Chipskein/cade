package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// moveImageTexts (schema version 18) links each stored image to its Image
// entity, stores its description and visible text as representations and
// rewrites its metadata without them, as a new database keeps it. It
// rewrites events, so the database is copied first.
func moveImageTexts(ctx context.Context, tx *sql.Tx) error {
	return forEachStoredEvent(ctx, tx, moveImageText)
}

func moveImageText(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	if ev.Source != event.SourceFile || ev.Image().SHA256 == "" {
		return nil
	}
	if err := relinkEntities(ctx, tx, eventID, ev); err != nil {
		return err
	}
	if err := representImage(ctx, tx, ev); err != nil {
		return err
	}
	metadata, err := encodeMetadata(ev.WithoutImageText().Metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET metadata = ? WHERE id = ?`, metadata, eventID); err != nil {
		return fmt.Errorf("remove the image text from the metadata of event %q: %w", ev.UID, err)
	}
	return nil
}
