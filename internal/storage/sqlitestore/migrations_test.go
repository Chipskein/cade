package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open(DriverName, "file:"+path+"?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func versionOf(t *testing.T, db *sql.DB) int {
	t.Helper()
	version, err := schemaVersion(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func TestNewDatabaseIsAtLatestVersion(t *testing.T) {
	store := openTestStore(t)
	if version := versionOf(t, store.db); version != schemaMigrations[len(schemaMigrations)-1].version {
		t.Fatalf("expected the latest version, got %d", version)
	}
}

// Databases created before versioning have the tables and user_version 0;
// they must reach version 1 with their events intact and no backup.
func TestUnversionedDatabaseKeepsItsEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	legacy := openRaw(t, path)
	legacy.Exec(createEventsTable)
	legacy.Exec(`INSERT INTO events (uid, occurred_at, source, content, metadata) VALUES ('a', 0, 'git', 'antigo', '{}')`)
	legacy.Close()
	var backups []string
	store, err := OpenWithHooks(context.Background(), path, Hooks{BackupCreated: func(p string) { backups = append(backups, p) }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, found, _ := store.StoredEvent(context.Background(), "a")
	if !found || stored.Content != "antigo" || versionOf(t, store.db) != 1 || len(backups) != 0 {
		t.Fatalf("expected the old event kept at version 1 without backup, got %+v found=%v backups=%v", stored, found, backups)
	}
}

func TestNewerDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	openRaw(t, path).Exec(`PRAGMA user_version = 99`)
	_, err := Open(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "version 99") || !strings.Contains(err.Error(), "update cade") {
		t.Fatalf("expected a refusal naming both versions, got %v", err)
	}
}

func addColumnStep(fail bool) migration {
	return migration{version: 2, description: "test column", backup: true, apply: func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE events ADD COLUMN extra TEXT`); err != nil {
			return err
		}
		if fail {
			return errors.New("falha simulada")
		}
		return nil
	}}
}

func storeAtVersionOne(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), nil)
	return store, path
}

func TestDataMigrationBacksUpFirst(t *testing.T) {
	store, path := storeAtVersionOne(t)
	var backup string
	steps := append(append([]migration{}, schemaMigrations...), addColumnStep(false))
	if err := migrate(context.Background(), store.db, path, steps, Hooks{BackupCreated: func(p string) { backup = p }}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if versionOf(t, store.db) != 2 || err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected version 2 and an owner-only backup, got v%d, %q (err %v)", versionOf(t, store.db), backup, err)
	}
	copied := openRaw(t, backup)
	var count int
	copied.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count)
	if count != 1 || versionOf(t, copied) != 1 {
		t.Fatalf("expected the backup at version 1 with the event, got v%d with %d events", versionOf(t, copied), count)
	}
}

func TestFailedMigrationKeepsPreviousVersion(t *testing.T) {
	store, path := storeAtVersionOne(t)
	steps := append(append([]migration{}, schemaMigrations...), addColumnStep(true))
	err := migrate(context.Background(), store.db, path, steps, Hooks{})
	if err == nil || !strings.Contains(err.Error(), "migration 2") || versionOf(t, store.db) != 1 {
		t.Fatalf("expected the failure reported and version 1 kept, got %v at v%d", err, versionOf(t, store.db))
	}
	if _, err := store.db.Exec(`SELECT extra FROM events`); err == nil {
		t.Fatal("expected the half-applied column rolled back")
	}
}

func TestMigrationsAreNumberedInOrder(t *testing.T) {
	for i, step := range schemaMigrations {
		if step.version != i+1 {
			t.Fatalf("migration %d has version %d; versions must be 1, 2, 3… with no gaps", i, step.version)
		}
	}
}
