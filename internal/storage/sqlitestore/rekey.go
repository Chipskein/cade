package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/chipskein/cade/internal/storage"
)

// rekeyBackupLayout names the copy taken before a rekey, like the copies
// taken before migrations.
const rekeyBackupLayout = "%s.before-rekey-%s"

// RekeyEvents implements storage.EventStore. Only the UID changes: the
// other tables reference events by row id, so chunks, vectors, people and
// file versions follow. A forgotten UID stays forgotten under its new UID,
// so an application changing its keys never brings back what the user
// asked to forget.
func (s *Store) RekeyEvents(ctx context.Context, changes map[string]string) (storage.RekeyReport, error) {
	if len(changes) == 0 {
		return storage.RekeyReport{}, nil
	}
	backupPath, err := s.backupBeforeRekey(ctx)
	if err != nil {
		return storage.RekeyReport{}, err
	}
	report := storage.RekeyReport{BackupPath: backupPath}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin rekey: %w", err)
	}
	defer tx.Rollback()
	for oldUID, newUID := range changes {
		if err := rekeyEvent(ctx, tx, oldUID, newUID, &report); err != nil {
			return report, err
		}
	}
	return report, tx.Commit()
}

func rekeyEvent(ctx context.Context, tx *sql.Tx, oldUID, newUID string, report *storage.RekeyReport) error {
	var taken bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE uid = ?)`, newUID).Scan(&taken); err != nil {
		return fmt.Errorf("look up UID %q: %w", newUID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO forgotten_events (uid, forgotten_at) SELECT ?, forgotten_at FROM forgotten_events WHERE uid = ?`, newUID, oldUID); err != nil {
		return fmt.Errorf("carry the forgotten mark of %q: %w", oldUID, err)
	}
	if taken {
		report.Conflicts++
		return nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE events SET uid = ? WHERE uid = ?`, newUID, oldUID)
	if err != nil {
		return fmt.Errorf("rekey %q to %q: %w", oldUID, newUID, err)
	}
	changed, err := result.RowsAffected()
	report.Rekeyed += int(changed)
	return err
}

// backupBeforeRekey copies the database next to itself (VACUUM INTO, which
// includes the WAL), owner-only like the original.
func (s *Store) backupBeforeRekey(ctx context.Context) (string, error) {
	var path string
	if err := s.db.QueryRowContext(ctx, `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
		return "", fmt.Errorf("locate the database file: %w", err)
	}
	backupPath := fmt.Sprintf(rekeyBackupLayout, path, time.Now().Format("20060102-150405"))
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		return "", fmt.Errorf("back up %q to %q before the rekey: %w", path, backupPath, err)
	}
	if err := os.Chmod(backupPath, privateFileMode); err != nil {
		return "", fmt.Errorf("restrict backup %q: %w", backupPath, err)
	}
	return backupPath, nil
}
