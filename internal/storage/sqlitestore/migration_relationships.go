package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// relateStoredEvents (schema version 16) creates the relationship table,
// relinks every stored event (version 15 did not link a commit's files)
// and stores the relations each states. Relinking keeps the ids of the
// entities that stay. Derived data only, so no copy; what the table holds
// is rebuilt, for a database whose version was set back.
func relateStoredEvents(ctx context.Context, tx *sql.Tx) error {
	if err := createRelationships(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM relationships`); err != nil {
		return fmt.Errorf("clear relationships before relating stored events: %w", err)
	}
	return forEachStoredEvent(ctx, tx, relinkAndRelate)
}

func relinkAndRelate(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	if err := relinkEntities(ctx, tx, eventID, ev); err != nil {
		return err
	}
	return relateEntities(ctx, tx, eventID, ev)
}
