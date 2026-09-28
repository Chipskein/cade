package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EmbeddingIndex = (*Store)(nil)

const (
	embeddingModelSettingKey       = "embedding_model"
	thresholdCalibrationSettingKey = "retrieval_threshold_calibration"
	reindexPendingSettingKey       = "reindex_pending"
)

// EmbeddingModel returns the model the vectors were computed with.
func (s *Store) EmbeddingModel(ctx context.Context) (string, error) {
	return s.setting(ctx, embeddingModelSettingKey)
}

// RecordEmbeddingModel names the model of the stored vectors.
func (s *Store) RecordEmbeddingModel(ctx context.Context, model string) error {
	return putSetting(ctx, s.db, embeddingModelSettingKey, model)
}

// ThresholdCalibration returns the recorded gates and their model, zero if
// none was recorded yet.
func (s *Store) ThresholdCalibration(ctx context.Context) (storage.ThresholdCalibration, error) {
	value, err := s.setting(ctx, thresholdCalibrationSettingKey)
	if err != nil || value == "" {
		return storage.ThresholdCalibration{}, err
	}
	var calibration storage.ThresholdCalibration
	if err := json.Unmarshal([]byte(value), &calibration); err != nil {
		return storage.ThresholdCalibration{}, fmt.Errorf("setting %q holds %q, expected a JSON threshold calibration: %w", thresholdCalibrationSettingKey, value, err)
	}
	return calibration, nil
}

// RecordThresholdCalibration records the gates and the model they are for.
func (s *Store) RecordThresholdCalibration(ctx context.Context, calibration storage.ThresholdCalibration) error {
	value, err := json.Marshal(calibration)
	if err != nil {
		return fmt.Errorf("encode threshold calibration %+v: %w", calibration, err)
	}
	return putSetting(ctx, s.db, thresholdCalibrationSettingKey, string(value))
}

// StartReindex drops the vector table (its dimension may change with the
// model) in one transaction with recording the model and the pending mark.
func (s *Store) StartReindex(ctx context.Context, model string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	statements := []string{`DROP TABLE IF EXISTS chunk_embeddings`, `INSERT INTO chunks_fts (chunks_fts) VALUES ('delete-all')`,
		`DELETE FROM chunks`, `DELETE FROM store_settings WHERE key = '` + dimensionsSettingKey + `'`}
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

// missingChunks selects events with text and no chunks: chunks and
// vectors are written together, so these are the ones to embed.
const missingChunks = `content != '' AND NOT EXISTS (SELECT 1 FROM chunks WHERE chunks.event_id = events.id)`

// EventsWithoutEmbedding returns the next events a reindex must embed.
func (s *Store) EventsWithoutEmbedding(ctx context.Context, limit int) ([]event.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE `+missingChunks+` ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query events without embedding: %w", err)
	}
	return collectEvents(rows)
}

// CountEventsWithoutEmbedding is what a reindex has left.
func (s *Store) CountEventsWithoutEmbedding(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE `+missingChunks).Scan(&count); err != nil {
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
		if err := insertChunks(ctx, tx, eventID, pair.Event, pair.Chunks); err != nil {
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
