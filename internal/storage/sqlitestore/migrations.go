package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// Schema changes are numbered migrations; PRAGMA user_version records the
// last one applied. Re-ingesting is not a way to change the schema: the
// Teams cache and browser history expire, so events lost to a rebuild do
// not come back.

// migration brings the database from version-1 to version.
type migration struct {
	version     int
	description string
	// backup copies the database first; set for steps that rewrite data.
	backup bool
	// compact reclaims the space of what the step deleted: dropping the
	// per-event vector table left ~500 MB of free pages in a 108k history.
	compact bool
	apply   func(ctx context.Context, tx *sql.Tx) error
}

// schemaMigrations lists every step, in order. Version 1 is the schema
// from before versioning; its IF NOT EXISTS makes it a no-op on databases
// created by earlier versions.
var schemaMigrations = []migration{
	{version: 1, description: "events and store_settings tables", apply: createBaseSchema},
	{version: 2, description: "Teams message text kept in metadata", backup: true, apply: keepTeamsText},
	{version: 3, description: "content_hash to reuse vectors of identical text", apply: addContentHash},
	{version: 4, description: "one event per file, versions in file_modifications", backup: true, apply: collapseFileVersions},
	{version: 5, description: "vectors per chunk instead of per event", backup: true, compact: true, apply: splitIntoChunks},
	{version: 6, description: "keyword index over chunks (FTS5)", apply: indexExistingChunks},
	{version: 7, description: "people index and message direction, for filters in SQL", apply: buildPeopleIndex},
	{version: 8, description: "redact known secrets and remove credential files", backup: true, compact: true, apply: redactStoredEvents},
	{version: 9, description: "remember individually forgotten event UIDs", apply: createForgottenEvents},
}

func createForgottenEvents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS forgotten_events (uid TEXT PRIMARY KEY, forgotten_at INTEGER NOT NULL)`)
	return err
}

// Hooks lets the caller report what opening the database did.
type Hooks struct {
	// BackupCreated receives the path of the copy made before a migration.
	BackupCreated func(backupPath string)
}

func createBaseSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, createEventsTable)
	return err
}

// migrate applies the pending steps, each in its own transaction together
// with the new user_version, so a failure leaves the last good version.
func migrate(ctx context.Context, db *sql.DB, path string, steps []migration, hooks Hooks) error {
	current, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}
	if latest := steps[len(steps)-1].version; current > latest {
		return fmt.Errorf("database %q is at schema version %d, newer than this cade supports (%d); update cade", path, current, latest)
	}
	backedUp, compact := false, false
	for _, step := range steps[current:] {
		if err := applyMigration(ctx, db, path, step, hooks, &backedUp); err != nil {
			return err
		}
		compact = compact || step.compact
	}
	if !compact {
		return nil
	}
	return (&Store{db: db}).compact(ctx)
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func applyMigration(ctx context.Context, db *sql.DB, path string, step migration, hooks Hooks, backedUp *bool) error {
	if err := backupIfNeeded(ctx, db, path, step, hooks, backedUp); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", step.version, err)
	}
	defer tx.Rollback()
	if err := step.apply(ctx, tx); err != nil {
		return fmt.Errorf("migration %d (%s): %w", step.version, step.description, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, step.version)); err != nil {
		return fmt.Errorf("record schema version %d: %w", step.version, err)
	}
	return tx.Commit()
}

// backupIfNeeded copies the database before the first data-rewriting step
// of this run, unless it holds no events yet (a new database has nothing to
// lose). One copy covers every later step: it already holds the state
// before all of them, and a second 500 MB copy would only fill the disk.
func backupIfNeeded(ctx context.Context, db *sql.DB, path string, step migration, hooks Hooks, backedUp *bool) error {
	if !step.backup || *backedUp {
		return nil
	}
	*backedUp = true
	var hasEvents bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events)`).Scan(&hasEvents); err != nil {
		return fmt.Errorf("check for events before migration %d: %w", step.version, err)
	}
	if !hasEvents {
		return nil
	}
	return backupBefore(ctx, db, path, step, hooks)
}

// backupBefore writes a consistent copy (VACUUM INTO includes what is
// still in the WAL) next to the database, owner-only like the original.
func backupBefore(ctx context.Context, db *sql.DB, path string, step migration, hooks Hooks) error {
	backupPath := fmt.Sprintf("%s.before-v%d-%s", path, step.version, time.Now().Format("20060102-150405"))
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		return fmt.Errorf("back up %q to %q before migration %d: %w", path, backupPath, step.version, err)
	}
	if err := os.Chmod(backupPath, privateFileMode); err != nil {
		return fmt.Errorf("restrict backup %q: %w", backupPath, err)
	}
	if hooks.BackupCreated != nil {
		hooks.BackupCreated(backupPath)
	}
	return nil
}
