// Package sqlitestore implements storage.EventStore on a local SQLite file
// with the sqlite-vec extension for vector search (RNF2.1).
package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// DriverName is the database/sql driver that has sqlite-vec loaded. Other
// packages that read SQLite files (e.g. browser history) open it by name.
const DriverName = "sqlite3"

func init() {
	sqlitevec.Auto()
}

// Store is the SQLite-backed storage.EventStore.
type Store struct {
	db *sql.DB
}

var _ storage.EventStore = (*Store)(nil)

// Open opens (creating if needed) the database file at path.
//
//	store, err := sqlitestore.Open(ctx, "~/.local/share/cade/cade.db")
//
// secure_delete zeroes the old text when an event is deleted or replaced
// (an edited message), instead of leaving it in free pages.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := restrictPermissions(path); err != nil {
		return nil, err
	}
	db, err := sql.Open(DriverName, "file:"+path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_secure_delete=on")
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	// One connection: SQLite serialises writes anyway, and this keeps the
	// lazily created vec0 table visible to every statement.
	db.SetMaxOpenConns(1)
	if err := createSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare database %q: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveEvent inserts the event and its embedding atomically.
func (s *Store) SaveEvent(ctx context.Context, ev event.Event, embedding []float32) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	eventID, inserted, err := insertEventRow(ctx, tx, ev)
	if err != nil || !inserted {
		return false, err
	}
	if err := insertEmbedding(ctx, tx, eventID, ev, embedding); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit event %q: %w", ev.UID, err)
	}
	return true, nil
}

func insertEventRow(ctx context.Context, tx *sql.Tx, ev event.Event) (int64, bool, error) {
	metadata, err := encodeMetadata(ev.Metadata)
	if err != nil {
		return 0, false, err
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO events (uid, occurred_at, source, content, metadata) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (uid) DO NOTHING`,
		ev.UID, toUnixMillis(ev.Timestamp), string(ev.Source), ev.Content, metadata)
	if err != nil {
		return 0, false, fmt.Errorf("insert event %q: %w", ev.UID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return 0, false, err
	}
	eventID, err := result.LastInsertId()
	return eventID, err == nil, err
}

func insertEmbedding(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event, embedding []float32) error {
	if len(embedding) == 0 {
		return nil
	}
	if err := ensureVectorTable(ctx, tx, len(embedding)); err != nil {
		return err
	}
	blob, err := sqlitevec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding of event %q: %w", ev.UID, err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO event_embeddings (event_id, embedding, source, occurred_at) VALUES (?, ?, ?, ?)`,
		eventID, blob, string(ev.Source), toUnixMillis(ev.Timestamp))
	if err != nil {
		return fmt.Errorf("insert embedding of event %q: %w", ev.UID, err)
	}
	return nil
}

// EventsBetween returns events with from <= timestamp < to, oldest first.
func (s *Store) EventsBetween(ctx context.Context, from, to time.Time) ([]event.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events
		 WHERE occurred_at >= ? AND occurred_at < ?
		 ORDER BY occurred_at, id`,
		toUnixMillis(from), toUnixMillis(to))
	if err != nil {
		return nil, fmt.Errorf("query events between %s and %s: %w", from, to, err)
	}
	return collectEvents(rows)
}
