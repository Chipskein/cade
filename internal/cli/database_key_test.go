package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestPlainDatabaseOpensWithoutLookingForAKey(t *testing.T) {
	world := newFakeWorld()
	world.keyVault.Failure = errors.New("must not be read")
	if code, _, stderr := world.run("timeline", "ontem"); code != 0 || world.openedKey != nil {
		t.Fatalf("code %d, key %x, stderr %q; want a plain open", code, world.openedKey, stderr)
	}
}

func TestEncryptedDatabaseOpensWithTheKeyFound(t *testing.T) {
	world := newFakeWorld()
	world.encrypted, world.keyVault.Key = true, []byte("the key")
	if code, _, stderr := world.run("timeline", "ontem"); code != 0 || string(world.openedKey) != "the key" {
		t.Fatalf("code %d, key %q, stderr %q; want the vault's key", code, world.openedKey, stderr)
	}
	if code, _, stderr := world.run("doctor"); string(world.openedKey) != "the key" {
		t.Fatalf("doctor: code %d, key %q, stderr %q; want the vault's key", code, world.openedKey, stderr)
	}
}

func TestEncryptedDatabaseWithoutAKeySaysWhereItLooked(t *testing.T) {
	world := newFakeWorld()
	world.encrypted, world.language = true, English
	code, _, stderr := world.run("timeline", "yesterday")
	if code == 0 || !strings.Contains(stderr, "is encrypted and its key was not found") || !strings.Contains(stderr, "looked in: the fake vault") {
		t.Errorf("code %d, stderr %q; want the places looked in", code, stderr)
	}
}

func TestRejectedKeySaysWhereItWasRead(t *testing.T) {
	world := newFakeWorld()
	world.encrypted, world.keyVault.Key, world.language = true, []byte("old key"), English
	world.openError = errors.New("the key given does not decrypt it")
	code, _, stderr := world.run("timeline", "yesterday")
	if code == 0 || !strings.Contains(stderr, "does not decrypt it (key read from the fake vault)") {
		t.Errorf("code %d, stderr %q; want the rejection with the key's source", code, stderr)
	}
}
