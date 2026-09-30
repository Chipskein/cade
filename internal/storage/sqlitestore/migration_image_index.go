package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// imageSHA256Expression reads an image's hash from an event's metadata.
// Queries must use this exact expression for SQLite to use the index.
var imageSHA256Expression = metadataField(event.ImageSHA256Key)

// indexImageHashes (schema version 10) lets ingestion find a description
// by the image's content, so a moved or renamed image is not described
// again (phase 19). The description itself lives in the event, and goes
// with it on `forget`.
func indexImageHashes(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS events_image_sha256 ON events (`+imageSHA256Expression+`)`); err != nil {
		return fmt.Errorf("index image hashes: %w", err)
	}
	return nil
}
