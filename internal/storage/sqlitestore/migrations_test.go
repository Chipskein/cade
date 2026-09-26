package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func TestNewDatabaseIsAtLatestVersionWithoutBackup(t *testing.T) {
	var backups []string
	store, err := OpenWithHooks(context.Background(), filepath.Join(t.TempDir(), "cade.db"), Hooks{BackupCreated: func(p string) { backups = append(backups, p) }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if version := versionOf(t, store.db); version != latestVersion() || len(backups) != 0 {
		t.Fatalf("expected the latest version and no backup, got v%d and %v", version, backups)
	}
}

func latestVersion() int {
	return schemaMigrations[len(schemaMigrations)-1].version
}

// Databases created before versioning have the tables and user_version 0;
// they must reach the latest version with their events intact, backed up
// first since a later step rewrites data.
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
	if !found || stored.Content != "antigo" || versionOf(t, store.db) != latestVersion() || len(backups) != 1 {
		t.Fatalf("expected the old event kept at the latest version with one backup, got %+v found=%v backups=%v", stored, found, backups)
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
	return migration{version: latestVersion() + 1, description: "test column", backup: true, apply: func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE events ADD COLUMN extra TEXT`); err != nil {
			return err
		}
		if fail {
			return errors.New("falha simulada")
		}
		return nil
	}}
}

func storeAtLatest(t *testing.T) (*Store, string) {
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
	store, path := storeAtLatest(t)
	var backup string
	steps := append(append([]migration{}, schemaMigrations...), addColumnStep(false))
	if err := migrate(context.Background(), store.db, path, steps, Hooks{BackupCreated: func(p string) { backup = p }}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if versionOf(t, store.db) != latestVersion()+1 || err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected the test version and an owner-only backup, got v%d, %q (err %v)", versionOf(t, store.db), backup, err)
	}
	copied := openRaw(t, backup)
	var count int
	copied.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count)
	if count != 1 || versionOf(t, copied) != latestVersion() {
		t.Fatalf("expected the backup at the previous version with the event, got v%d with %d events", versionOf(t, copied), count)
	}
}

func TestFailedMigrationKeepsPreviousVersion(t *testing.T) {
	store, path := storeAtLatest(t)
	steps := append(append([]migration{}, schemaMigrations...), addColumnStep(true))
	err := migrate(context.Background(), store.db, path, steps, Hooks{})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("migration %d", latestVersion()+1)) || versionOf(t, store.db) != latestVersion() {
		t.Fatalf("expected the failure reported and the previous version kept, got %v at v%d", err, versionOf(t, store.db))
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

func TestKeepTeamsTextRecoversTextOfOldEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	legacy := openRaw(t, path)
	legacy.Exec(createEventsTable)
	message := event.Message{Sender: "Ana", Conversation: "Ana, Eu", Kind: event.KindChat, MessageID: "1", Text: "deploy\nàs 19h"}
	withoutText := message.Metadata()
	delete(withoutText, "text")
	withoutText["extra"] = "mantido"
	encoded, _ := encodeMetadata(withoutText)
	legacy.Exec(`INSERT INTO events (uid, occurred_at, source, content, metadata) VALUES ('m', 0, 'teams', ?, ?)`, message.Content(), encoded)
	legacy.Exec(`INSERT INTO events (uid, occurred_at, source, content, metadata) VALUES ('old', 0, 'teams', 'Ana: layout antigo', '{"sender":"Ana"}')`)
	legacy.Close()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recovered, _, _ := store.StoredEvent(context.Background(), "m")
	old, _, _ := store.StoredEvent(context.Background(), "old")
	if recovered.Message().Text != "deploy\nàs 19h" || recovered.Metadata["extra"] != "mantido" || recovered.Content != message.Content() {
		t.Fatalf("expected the text recovered and other keys kept, got %+v", recovered.Metadata)
	}
	if old.Message().Text != "" || old.Content != "Ana: layout antigo" {
		t.Fatalf("expected the old layout left untouched, got %+v", old)
	}
}
