package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
)

// linkStoredEvents (schema version 15) creates the entity tables and links
// every stored event with the same entity.RefsOf ingestion uses, so a
// migrated database holds the entities a new one would. What the tables
// already hold is derived, so it is rebuilt: a database whose version was
// set back (as tests of earlier steps do) migrates again. Events are not
// rewritten; the backup is there because the issue (#20) asks for one.
func linkStoredEvents(ctx context.Context, tx *sql.Tx) error {
	if err := createEntities(ctx, tx); err != nil {
		return err
	}
	for _, table := range []string{"event_entities", "entities"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return fmt.Errorf("clear %s before linking stored events: %w", table, err)
		}
	}
	return forEachStoredEvent(ctx, tx, linkEntities)
}
