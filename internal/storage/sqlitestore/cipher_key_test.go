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

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

// cipherMarker is text an encrypted file must not show in any of its bytes.
const cipherMarker = "MARKER-111-plaintext"

var (
	testCipherKey  = CipherKey(bytes.Repeat([]byte{0x11}, 32))
	otherCipherKey = CipherKey(bytes.Repeat([]byte{0x22}, 32))
)

// openKeyedStore creates an encrypted database holding one event with the
// marker, and returns it open.
func openKeyedStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := OpenWithKey(context.Background(), path, testCipherKey, Hooks{})
	testcheck.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	ev := sampleEvent("marked", event.SourceTeams, 0)
	ev.Content = "the deploy note " + cipherMarker
	mustSave(t, store, ev, []float32{1, 0})
	return store
}

func assertNoMarker(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	testcheck.NoError(t, err)
	if len(content) == 0 || bytes.Contains(content, []byte(cipherMarker)) || bytes.HasPrefix(content, []byte(plainHeader)) {
		t.Errorf("%s (%d bytes) is readable or empty, want it encrypted", filepath.Base(path), len(content))
	}
}

func TestKeyedDatabaseEncryptsTheFileTheWALAndTheBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store := openKeyedStore(t, path)
	assertNoMarker(t, path+"-wal")
	var backup string
	step := schemaMigrations[len(schemaMigrations)-1]
	testcheck.NoError(t, backupBefore(context.Background(), store.db, path, step, Hooks{BackupCreated: func(p string) { backup = p }}))
	assertNoMarker(t, backup)
	testcheck.NoError(t, store.Close())
	assertNoMarker(t, path)
}

func TestKeyedDatabaseReopensWithItsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	testcheck.NoError(t, openKeyedStore(t, path).Close())
	reopened, err := OpenWithKey(context.Background(), path, testCipherKey, Hooks{})
	testcheck.NoError(t, err)
	defer reopened.Close()
	events, err := reopened.EventsBetween(context.Background(), baseTime, baseTime.Add(time.Hour))
	if err != nil || len(events) != 1 || !strings.Contains(events[0].Content, cipherMarker) {
		t.Fatalf("events = %+v, %v; want the marked event back", events, err)
	}
	state, err := InspectWithKey(context.Background(), path, testCipherKey)
	if err != nil || !state.Exists || state.SchemaVersion != latestVersion() {
		t.Errorf("inspect = %+v, %v; want the current schema", state, err)
	}
}

func TestOpenRejectsAKeyThatDoesNotMatchTheFile(t *testing.T) {
	dir := t.TempDir()
	encrypted, plain := filepath.Join(dir, "encrypted.db"), filepath.Join(dir, "plain.db")
	testcheck.NoError(t, openKeyedStore(t, encrypted).Close())
	plainStore, err := Open(context.Background(), plain)
	testcheck.NoError(t, err)
	testcheck.NoError(t, plainStore.Close())
	cases := []struct {
		name, path string
		key        CipherKey
		want       string
	}{
		{"missing key", encrypted, nil, "no key was given"},
		{"wrong key", encrypted, otherCipherKey, "does not decrypt it"},
		{"unneeded key", plain, testCipherKey, "is in plain text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before, err := os.ReadFile(c.path)
			testcheck.NoError(t, err)
			_, err = OpenWithKey(context.Background(), c.path, c.key, Hooks{})
			if !errors.Is(err, ErrCipherKeyRejected) || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), c.path) {
				t.Errorf("err = %v, want ErrCipherKeyRejected naming %s and saying %q", err, c.path, c.want)
			}
			if after, _ := os.ReadFile(c.path); !bytes.Equal(before, after) {
				t.Errorf("the rejected open changed %s", c.path)
			}
		})
	}
}

func TestDatabaseDSNAddsTheKeyOnlyWhenGiven(t *testing.T) {
	if got := databaseDSN("/x/cade.db", readOnlyParams, nil); got != "file:/x/cade.db?mode=ro" {
		t.Errorf("plain DSN = %q, want the params alone", got)
	}
	got := databaseDSN("/x/cade.db", readOnlyParams, CipherKey{0xab, 0x01})
	if got != "file:/x/cade.db?mode=ro&cipher=chacha20&hexkey=ab01&_cache_size=-262144" {
		t.Errorf("keyed DSN = %q, want the cipher, the hex key and the larger cache", got)
	}
}

func TestExplainOpenFailureReportsAnUnreadableHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "short.db")
	testcheck.NoError(t, os.WriteFile(path, []byte("SQL"), 0o600))
	_, err := openKeyed(context.Background(), path, readOnlyParams, nil)
	if err == nil || !strings.Contains(err.Error(), "reading its header") {
		t.Errorf("err = %v, want it to say the header could not be read", err)
	}
}
