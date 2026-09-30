package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/chipskein/cade/internal/storage"
)

// Inspect reads the database at path read-only, for `cade doctor`: unlike
// Open it applies no migration, makes no copy and creates no file. A
// missing database is not an error.
//
//	state, err := sqlitestore.Inspect(ctx, "/home/me/.local/share/cade/cade.db")
func Inspect(ctx context.Context, path string) (storage.DatabaseState, error) {
	state := storage.DatabaseState{LatestSchemaVersion: schemaMigrations[len(schemaMigrations)-1].version}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		state.FTS5, err = inMemoryFTS5(ctx)
		return state, err
	}
	if err != nil {
		return state, fmt.Errorf("stat database %q: %w", path, err)
	}
	db, err := sql.Open(DriverName, "file:"+path+"?mode=ro")
	if err != nil {
		return state, fmt.Errorf("open database %q read-only: %w", path, err)
	}
	defer db.Close()
	state.Exists, state.SizeBytes = true, info.Size()
	if err := readDatabaseState(ctx, db, &state); err != nil {
		return state, fmt.Errorf("inspect database %q: %w", path, err)
	}
	return state, nil
}

func inMemoryFTS5(ctx context.Context) (bool, error) {
	db, err := sql.Open(DriverName, ":memory:")
	if err != nil {
		return false, fmt.Errorf("open in-memory SQLite: %w", err)
	}
	defer db.Close()
	return fts5Enabled(ctx, db)
}

// readDatabaseState fills state; a database at version 0 (just created,
// or from before versioning) has no settings to read yet.
func readDatabaseState(ctx context.Context, db *sql.DB, state *storage.DatabaseState) error {
	var err error
	if state.FTS5, err = fts5Enabled(ctx, db); err != nil {
		return err
	}
	if state.SchemaVersion, err = schemaVersion(ctx, db); err != nil || state.SchemaVersion == 0 {
		return err
	}
	if state.MigrationBackup, err = pendingMigrationBackup(ctx, db, state.SchemaVersion); err != nil {
		return err
	}
	store := &Store{db: db}
	if state.EmbeddingModel, err = store.EmbeddingModel(ctx); err != nil {
		return err
	}
	if state.ThresholdCalibration, err = store.ThresholdCalibration(ctx); err != nil {
		return err
	}
	state.ReindexPending, err = store.ReindexPending(ctx)
	return err
}

// pendingMigrationBackup mirrors backupIfNeeded: a copy is made when a
// pending step rewrites data and the database holds events.
func pendingMigrationBackup(ctx context.Context, db *sql.DB, version int) (bool, error) {
	pending := schemaMigrations[min(version, len(schemaMigrations)):]
	rewrites := false
	for _, step := range pending {
		rewrites = rewrites || step.backup
	}
	if !rewrites {
		return false, nil
	}
	var hasEvents bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events)`).Scan(&hasEvents); err != nil {
		return false, fmt.Errorf("check for events: %w", err)
	}
	return hasEvents, nil
}
