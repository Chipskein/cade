package sqlitestore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/dbcopy"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

// plainStoreWithMarker creates a plain database holding the marked event,
// with the event still in the WAL, and closes it.
func plainStoreWithMarker(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	ev := sampleEvent("marked", event.SourceTeams, 0)
	ev.Content = "the deploy note " + cipherMarker
	mustSave(t, store, ev, []float32{1, 0})
	testcheck.NoError(t, store.Close())
	return path
}

func assertMarkedEvent(t *testing.T, path string, key CipherKey) {
	t.Helper()
	store, err := OpenWithKey(context.Background(), path, key, Hooks{})
	testcheck.NoError(t, err)
	defer store.Close()
	events, err := store.EventsBetween(context.Background(), baseTime, baseTime.Add(time.Hour))
	if err != nil || len(events) != 1 || !strings.Contains(events[0].Content, cipherMarker) {
		t.Fatalf("events = %+v, %v; want the marked event", events, err)
	}
}

func TestReplaceWithKeyEncryptsAPlainDatabase(t *testing.T) {
	path := plainStoreWithMarker(t)
	backup := dbcopy.BeforeEncrypt(path, baseTime)
	testcheck.NoError(t, ReplaceWithKey(context.Background(), path, nil, testCipherKey, backup))
	assertNoMarker(t, path)
	assertNoMarker(t, backup)
	testcheck.OwnerOnly(t, path)
	testcheck.OwnerOnly(t, backup)
	if _, err := os.Stat(dbcopy.Converting(path)); !os.IsNotExist(err) {
		t.Errorf("the partial file is left (stat err %v)", err)
	}
	if encrypted, err := IsEncrypted(path); err != nil || !encrypted {
		t.Errorf("IsEncrypted = %v, %v; want true", encrypted, err)
	}
	assertMarkedEvent(t, path, testCipherKey)
}

func TestReplaceWithKeyDecryptsAndRekeys(t *testing.T) {
	path := plainStoreWithMarker(t)
	testcheck.NoError(t, ReplaceWithKey(context.Background(), path, nil, testCipherKey, dbcopy.BeforeEncrypt(path, baseTime)))
	testcheck.NoError(t, ReplaceWithKey(context.Background(), path, testCipherKey, otherCipherKey, dbcopy.BeforeRekey(path, baseTime)))
	assertMarkedEvent(t, path, otherCipherKey)
	backup := dbcopy.BeforeDecrypt(path, baseTime)
	testcheck.NoError(t, ReplaceWithKey(context.Background(), path, otherCipherKey, nil, backup))
	for _, file := range []string{path, backup} {
		if encrypted, err := IsEncrypted(file); err != nil || encrypted {
			t.Errorf("IsEncrypted(%s) = %v, %v; want plain", filepath.Base(file), encrypted, err)
		}
	}
	assertMarkedEvent(t, path, nil)
}

func TestReplaceWithKeyLeavesTheDatabaseOnAWrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	testcheck.NoError(t, openKeyedStore(t, path).Close())
	before, err := os.ReadFile(path)
	testcheck.NoError(t, err)
	backup := dbcopy.BeforeDecrypt(path, baseTime)
	if err := ReplaceWithKey(context.Background(), path, otherCipherKey, nil, backup); !errors.Is(err, ErrCipherKeyRejected) {
		t.Fatalf("err = %v, want ErrCipherKeyRejected", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("the rejected replace changed the database")
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Errorf("a backup was written (stat err %v), want none", err)
	}
}

func TestIsEncryptedOfMissingAndEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.db")
	testcheck.NoError(t, os.WriteFile(empty, nil, 0o600))
	for _, path := range []string{filepath.Join(dir, "missing.db"), empty} {
		if encrypted, err := IsEncrypted(path); err != nil || encrypted {
			t.Errorf("IsEncrypted(%s) = %v, %v; want false", filepath.Base(path), encrypted, err)
		}
	}
}

func TestInspectWithoutTheKeyReportsAnUnreadableDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	testcheck.NoError(t, openKeyedStore(t, path).Close())
	state, err := Inspect(context.Background(), path)
	if err != nil || !state.Exists || !state.Encrypted || !state.Unreadable || !state.FTS5 || state.SizeBytes == 0 || state.SchemaVersion != 0 {
		t.Errorf("state = %+v, %v; want an existing, encrypted, unreadable database", state, err)
	}
	if state, err := InspectWithKey(context.Background(), path, testCipherKey); err != nil || !state.Encrypted || state.Unreadable || state.SchemaVersion != latestVersion() {
		t.Errorf("with the key: state = %+v, %v; want it read", state, err)
	}
	if _, err := InspectWithKey(context.Background(), path, otherCipherKey); !errors.Is(err, ErrCipherKeyRejected) {
		t.Errorf("with another key: err = %v, want ErrCipherKeyRejected", err)
	}
}
