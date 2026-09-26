package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EmbeddingIndex = (*Store)(nil)

const (
	embeddingModelSettingKey = "embedding_model"
	reindexPendingSettingKey = "reindex_pending"
)

// EmbeddingModel returns the model the vectors were computed with.
func (s *Store) EmbeddingModel(ctx context.Context) (string, error) {
	return s.setting(ctx, embeddingModelSettingKey)
}

// RecordEmbeddingModel names the model of the stored vectors.
func (s *Store) RecordEmbeddingModel(ctx context.Context, model string) error {
	return putSetting(ctx, s.db, embeddingModelSettingKey, model)
}

// StartReindex drops the vector table (its dimension may change with the
// model) in one transaction with recording the model and the pending mark.
func (s *Store) StartReindex(ctx context.Context, model string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	statements := []string{`DROP TABLE IF EXISTS event_embeddings`, `DELETE FROM store_settings WHERE key = '` + dimensionsSettingKey + `'`}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("drop vectors for reindex: %w", err)
		}
	}
	for key, value := range map[string]string{embeddingModelSettingKey: model, reindexPendingSettingKey: "1"} {
		if err := putSetting(ctx, tx, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReindexPending reports a rebuild that started and has not finished.
func (s *Store) ReindexPending(ctx context.Context) (bool, error) {
	value, err := s.setting(ctx, reindexPendingSettingKey)
	return value != "", err
}

// FinishReindex clears the pending mark.
func (s *Store) FinishReindex(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM store_settings WHERE key = ?`, reindexPendingSettingKey)
	return err
}

// missingEmbeddingFilter selects events with text and no vector; without
// a vector table, every event with text.
func (s *Store) missingEmbeddingFilter(ctx context.Context) (string, error) {
	ready, err := s.vectorTableExists(ctx)
	if err != nil || !ready {
		return `content != ''`, err
	}
	return `content != '' AND id NOT IN (SELECT event_id FROM event_embeddings)`, nil
}

// EventsWithoutEmbedding returns the next events a reindex must embed.
func (s *Store) EventsWithoutEmbedding(ctx context.Context, limit int) ([]event.Event, error) {
	filter, err := s.missingEmbeddingFilter(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE `+filter+` ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query events without embedding: %w", err)
	}
	return collectEvents(rows)
}

// CountEventsWithoutEmbedding is what a reindex has left.
func (s *Store) CountEventsWithoutEmbedding(ctx context.Context) (int, error) {
	filter, err := s.missingEmbeddingFilter(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE `+filter).Scan(&count); err != nil {
		return 0, fmt.Errorf("count events without embedding: %w", err)
	}
	return count, nil
}

// SaveEmbeddings stores a batch of vectors in one transaction, so an
// interrupted reindex resumes after the last complete batch.
func (s *Store) SaveEmbeddings(ctx context.Context, embeddings []storage.EventEmbedding) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	for _, pair := range embeddings {
		var eventID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM events WHERE uid = ?`, pair.Event.UID).Scan(&eventID); err != nil {
			return fmt.Errorf("find event %q, expected it to be stored: %w", pair.Event.UID, err)
		}
		if err := insertEmbedding(ctx, tx, eventID, pair.Event, pair.Vector); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) setting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM store_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %q: %w", key, err)
	}
	return value, nil
}

// settingWriter is satisfied by both *sql.DB and *sql.Tx.
type settingWriter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func putSetting(ctx context.Context, writer settingWriter, key, value string) error {
	_, err := writer.ExecContext(ctx, `INSERT INTO store_settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("write setting %q: %w", key, err)
	}
	return nil
}
