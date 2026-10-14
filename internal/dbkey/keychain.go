package dbkey

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// securityProgram is macOS's command-line front end to the Keychain;
	// it ships with the system, so the key needs no cgo binding.
	securityProgram = "/usr/bin/security"
	// The item is a generic password, service and account below; the
	// value is the key in hex, since -w prints it as text.
	keychainService = "cade-database"
	keychainAccount = "cade"
	// keychainItemNotFound is security's exit code for a missing item
	// (errSecItemNotFound).
	keychainItemNotFound = 44
)

// Keychain keeps the key in the user's login Keychain, which macOS
// unlocks at login, so the ingest agent can read it too.
type Keychain struct {
	Runner CommandRunner
}

var _ KeyVault = Keychain{}

// exitCoder is what exec.ExitError (and the tests' fake) tells apart: the
// tool's exit code.
type exitCoder interface {
	ExitCode() int
}

func exitedWith(err error, code int) bool {
	var coder exitCoder
	return errors.As(err, &coder) && coder.ExitCode() == code
}

// Load reads the item; a missing item is ErrNoKey.
func (k Keychain) Load(ctx context.Context) ([]byte, error) {
	output, err := k.Runner.Run(ctx, nil, securityProgram, "find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w")
	if exitedWith(err, keychainItemNotFound) {
		return nil, fmt.Errorf("no Keychain item %q: %w", keychainService, ErrNoKey)
	}
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(string(bytes.TrimSpace(output)))
	if err != nil {
		return nil, fmt.Errorf("Keychain item %q: %d characters that are not hex; want the key cade encrypt stored", keychainService, len(bytes.TrimSpace(output)))
	}
	return key, nil
}

// Save adds or replaces (-U) the item. The command goes to security's
// interactive mode on stdin, so the key is not in any process's arguments.
func (k Keychain) Save(ctx context.Context, key []byte) error {
	command := strings.Join([]string{"add-generic-password", "-U", "-s", keychainService, "-a", keychainAccount, "-w", hex.EncodeToString(key)}, " ")
	_, err := k.Runner.Run(ctx, []byte(command+"\n"), securityProgram, "-i")
	return err
}

// Remove deletes the item; a missing one is not an error.
func (k Keychain) Remove(ctx context.Context) error {
	_, err := k.Runner.Run(ctx, nil, securityProgram, "delete-generic-password", "-s", keychainService, "-a", keychainAccount)
	if err != nil && !exitedWith(err, keychainItemNotFound) {
		return err
	}
	return nil
}

func (k Keychain) Location() string {
	return "the login Keychain, item " + keychainService
}
