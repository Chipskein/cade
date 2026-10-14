package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/dbkey"
	"github.com/chipskein/cade/internal/rootfs"
)

const encryptedDatabasePath = "/data/cade.db"

// generatedKey is what the fake Random yields for a new key.
var generatedKey = bytes.Repeat([]byte("k"), dbkey.KeySize)

// newEncryptionWorld has a plain database at encryptedDatabasePath, in
// English, so the tests read the messages.
func newEncryptionWorld() *fakeWorld {
	world := newFakeWorld()
	world.cfg.DatabasePath, world.language = encryptedDatabasePath, English
	world.files.AddFile(encryptedDatabasePath, "plain")
	return world
}

func TestEncryptStoresTheKeyThenRewritesTheDatabase(t *testing.T) {
	world := newEncryptionWorld()
	code, stdout, stderr := world.run("encrypt", "--yes")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	want := keyReplacement{path: encryptedDatabasePath, backup: "/data/cade.db.before-encrypt-20260926-120000", newKey: generatedKey}
	if len(world.replacements) != 1 || !keyReplacementsEqual(world.replacements[0], want) || !bytes.Equal(world.keyVault.Key, generatedKey) {
		t.Fatalf("replacements %+v, vault %x; want the new key stored and the database rewritten", world.replacements, world.keyVault.Key)
	}
	for _, line := range []string{"Database encrypted: /data/cade.db", "Key stored in the fake vault.", dbkey.FormatRecoveryKey(generatedKey), "CADE_DB_RECOVERY_KEY"} {
		if !strings.Contains(stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, stdout)
		}
	}
	if world.runLock.Acquired != 1 || world.runLock.Held {
		t.Errorf("lock acquired %d, held %v; want it taken and released", world.runLock.Acquired, world.runLock.Held)
	}
}

func keyReplacementsEqual(a, b keyReplacement) bool {
	return a.path == b.path && a.backup == b.backup && bytes.Equal(a.oldKey, b.oldKey) && bytes.Equal(a.newKey, b.newKey)
}

func TestEncryptWithoutAVaultUsesThePassphrase(t *testing.T) {
	world := newEncryptionWorld()
	world.noVault, world.env[string(dbkey.PassphraseVariable)] = true, "correct horse battery"
	code, stdout, stderr := world.run("encrypt", "--yes")
	if code != 0 || len(world.replacements) != 1 || string(world.replacements[0].newKey) != "correct horse battery" {
		t.Fatalf("code %d, replacements %+v, stderr %q; want the passphrase as the key", code, world.replacements, stderr)
	}
	if !strings.Contains(stdout, "The passphrase is the key") || strings.Contains(stdout, "Recovery key") {
		t.Errorf("stdout = %q, want the passphrase note and no recovery key", stdout)
	}
}

func TestEncryptAsksForAPassphraseTwice(t *testing.T) {
	world := newEncryptionWorld()
	world.noVault = true
	world.passphrases.Lines = []string{"correct horse battery", "correct horse battery"}
	if code, _, stderr := world.run("encrypt", "--yes"); code != 0 || len(world.passphrases.Prompts) != 2 {
		t.Fatalf("code %d, prompts %q, stderr %q; want the passphrase asked twice", code, world.passphrases.Prompts, stderr)
	}
}

func TestEncryptCreatesAMissingDatabaseEncrypted(t *testing.T) {
	world := newEncryptionWorld()
	delete(world.files.MapFS, rootfs.Name(encryptedDatabasePath))
	code, stdout, stderr := world.run("encrypt", "--yes")
	if code != 0 || len(world.replacements) != 0 || !bytes.Equal(world.openedKey, generatedKey) || strings.Contains(stdout, "Copy from before") {
		t.Fatalf("code %d, replacements %+v, opened with %x, stderr %q; want the store created with the key", code, world.replacements, world.openedKey, stderr)
	}
}

func TestEncryptListsThePlainCopiesLeft(t *testing.T) {
	world := newEncryptionWorld()
	world.files.AddFile("/data/cade.db.before-v18-20261001-000000", "plain copy")
	world.files.AddFile("/data/cade.db.before-rekey-20261002-000000", "encrypted copy")
	world.encryptedCopies = map[string]bool{"/data/cade.db.before-v18-20261001-000000": false, "/data/cade.db.before-rekey-20261002-000000": true}
	_, stdout, _ := world.run("encrypt", "--yes")
	if !strings.Contains(stdout, "still in plain text") || !strings.Contains(stdout, "before-v18") || strings.Contains(stdout, "before-rekey") {
		t.Errorf("stdout = %q, want the plain copy listed and not the encrypted one", stdout)
	}
}

func TestEncryptRefuses(t *testing.T) {
	cases := map[string]struct {
		setup func(*fakeWorld)
		args  []string
		want  string
	}{
		"already encrypted": {func(w *fakeWorld) { w.encrypted = true }, []string{"--yes"}, "already encrypted"},
		"unconfirmed":       {func(*fakeWorld) {}, nil, "use --yes outside a terminal"},
		"ingest running":    {func(w *fakeWorld) { w.runLock.Held = true }, []string{"--yes"}, "another ingestion is running"},
		"vault failing":     {func(w *fakeWorld) { w.keyVault.Failure = errors.New("locked") }, []string{"--yes"}, "store the key in the fake vault: locked"},
		"an argument":       {func(*fakeWorld) {}, []string{"now"}, "takes no arguments"},
	}
	for name, c := range cases {
		world := newEncryptionWorld()
		c.setup(world)
		code, _, stderr := world.run(append([]string{"encrypt"}, c.args...)...)
		if code == 0 || !strings.Contains(stderr, c.want) || len(world.replacements) != 0 {
			t.Errorf("%s: code %d, replacements %+v, stderr %q; want a refusal saying %q", name, code, world.replacements, stderr, c.want)
		}
	}
}

func TestDecryptRewritesInPlainTextThenRemovesTheKey(t *testing.T) {
	world := newEncryptionWorld()
	world.encrypted, world.keyVault.Key = true, generatedKey
	code, stdout, stderr := world.run("decrypt", "--yes")
	want := keyReplacement{path: encryptedDatabasePath, backup: "/data/cade.db.before-decrypt-20260926-120000", oldKey: generatedKey}
	if code != 0 || len(world.replacements) != 1 || !keyReplacementsEqual(world.replacements[0], want) || world.keyVault.Key != nil {
		t.Fatalf("code %d, replacements %+v, vault %x, stderr %q; want a plain rewrite and the key removed", code, world.replacements, world.keyVault.Key, stderr)
	}
	if !strings.Contains(stdout, "Database in plain text: /data/cade.db") || !strings.Contains(stdout, "Key removed from the fake vault.") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestFailedDecryptKeepsTheKey(t *testing.T) {
	world := newEncryptionWorld()
	world.encrypted, world.keyVault.Key, world.replaceError = true, generatedKey, errors.New("disk full")
	code, _, stderr := world.run("decrypt", "--yes")
	if code == 0 || !strings.Contains(stderr, "disk full (key read from the fake vault)") || !bytes.Equal(world.keyVault.Key, generatedKey) {
		t.Errorf("code %d, vault %x, stderr %q; want the failure and the key kept", code, world.keyVault.Key, stderr)
	}
}

func TestDecryptRefusesAPlainDatabaseAndListsEncryptedCopies(t *testing.T) {
	world := newEncryptionWorld()
	if code, _, stderr := world.run("decrypt", "--yes"); code == 0 || !strings.Contains(stderr, "is not encrypted") {
		t.Errorf("plain database: code %d, stderr %q; want a refusal", code, stderr)
	}
	world.encrypted, world.keyVault.Key = true, generatedKey
	world.files.AddFile("/data/cade.db.before-encrypt-20261001-000000", "encrypted copy")
	world.encryptedCopies = map[string]bool{"/data/cade.db.before-encrypt-20261001-000000": true}
	if _, stdout, _ := world.run("decrypt", "--yes"); !strings.Contains(stdout, "only with the recovery key") || !strings.Contains(stdout, "before-encrypt-20261001") {
		t.Errorf("stdout = %q, want the encrypted copy listed", stdout)
	}
}
