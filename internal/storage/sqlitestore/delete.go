package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// DeleteSource removes the events of source and their embeddings in one
// transaction, so an interruption never leaves embeddings without events,
// then compacts the file so the deleted text leaves the disk.
func (s *Store) DeleteSource(ctx context.Context, source event.Source) (int, error) {
	removed, err := s.deleteSourceRows(ctx, source)
	if err != nil {
		return 0, err
	}
	return removed, s.compact(ctx)
}

// DeleteEvent permanently removes one event and records only its UID for reingest suppression.
func (s *Store) DeleteEvent(ctx context.Context, uid string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	var source string
	err = tx.QueryRowContext(ctx, `SELECT source FROM events WHERE uid = ?`, uid).Scan(&source)
	if err == sql.ErrNoRows {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	key, err := storedTextKeyOfUID(ctx, tx, uid)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_identifiers_fts WHERE rowid IN (SELECT id FROM events WHERE uid=?)`, uid); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_people WHERE event_id IN (SELECT id FROM events WHERE uid=?)`, uid); err != nil {
		return false, err
	}
	if source == string(event.SourceFile) {
		var path string
		if err := tx.QueryRowContext(ctx, `SELECT json_extract(metadata, '$.path') FROM events WHERE uid=?`, uid).Scan(&path); err == nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM file_modifications WHERE path=?`, path); err != nil {
				return false, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE uid=?`, uid); err != nil {
		return false, err
	}
	if err := releaseText(ctx, tx, key); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO forgotten_events(uid, forgotten_at) VALUES (?, ?)`, uid, time.Now().UnixMilli()); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, s.compact(ctx)
}

func (s *Store) IsForgotten(ctx context.Context, uid string) (bool, error) {
	return isForgotten(ctx, s.db, uid)
}

func isForgotten(ctx context.Context, querier queryer, uid string) (bool, error) {
	var found bool
	err := querier.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM forgotten_events WHERE uid=?)`, uid).Scan(&found)
	return found, err
}

func (s *Store) EventsContaining(ctx context.Context, text string, filter storage.EventFilter) ([]event.Event, error) {
	from, to := timeBounds(filter.From, filter.To)
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE instr(lower(content), lower(?))>0 AND (?='' OR source=?) AND occurred_at>=? AND occurred_at<? ORDER BY occurred_at,id`, text, filter.Source, filter.Source, from, to)
	if err != nil {
		return nil, err
	}
	return collectEvents(rows)
}

func (s *Store) DeleteBefore(ctx context.Context, source event.Source, before time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid FROM events WHERE source=? AND occurred_at<?`, string(source), toUnixMillis(before))
	if err != nil {
		return 0, err
	}
	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return 0, err
		}
		uids = append(uids, uid)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	removed := 0
	for _, uid := range uids {
		deleted, err := s.DeleteEvent(ctx, uid)
		if err != nil {
			return removed, err
		}
		if deleted {
			removed++
		}
	}
	return removed, nil
}

func (s *Store) deleteSourceRows(ctx context.Context, source event.Source) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM forgotten_events`); err != nil {
		return 0, err
	}
	if err := deleteSourceEmbeddings(ctx, tx, source); err != nil {
		return 0, err
	}
	if err := deleteSourceHistory(ctx, tx, source); err != nil {
		return 0, err
	}
	if err := unindexSource(ctx, tx, source); err != nil {
		return 0, err
	}
	if err := unindexSourceIdentifiers(ctx, tx, source); err != nil {
		return 0, err
	}
	if err := deleteSourceChunks(ctx, tx, source); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM events WHERE source = ?`, string(source))
	if err != nil {
		return 0, fmt.Errorf("delete %s events: %w", source, err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(removed), tx.Commit()
}

// deleteSourceEmbeddings deletes through the vec0 metadata column, which
// avoids a subquery join that vec0 cannot plan.
func deleteSourceEmbeddings(ctx context.Context, tx *sql.Tx, source event.Source) error {
	_, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE source = ?`, string(source)); err != nil {
		return fmt.Errorf("delete %s embeddings: %w", source, err)
	}
	return nil
}

// deleteSourceHistory removes what a source keeps outside events: the
// file edit history holds paths, which `forget file` must erase too.
func deleteSourceHistory(ctx context.Context, tx *sql.Tx, source event.Source) error {
	if source != event.SourceFile {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_modifications`); err != nil {
		return fmt.Errorf("delete file modifications: %w", err)
	}
	return nil
}
