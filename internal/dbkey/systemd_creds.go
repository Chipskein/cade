package dbkey

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"

	"github.com/chipskein/cade/internal/privatefile"
)

const (
	systemdCredsProgram = "systemd-creds"
	// credentialName is sealed into the file: systemd-creds refuses to
	// decrypt it under another name, so the file cannot be passed off as
	// another service's credential.
	credentialName = "--name=cade-db"
	// systemdCredsUserVersion is the first systemd whose systemd-creds has
	// --user, which needs no root and works without a graphical session,
	// so also under cade-ingest.timer (#57).
	systemdCredsUserVersion = 256
	credentialFileMode      = 0o600
	stdStream               = "-"
	partialCredentialSuffix = ".part"
)

var systemdVersionPattern = regexp.MustCompile(`^systemd (\d+)`)

// SystemdCreds keeps the key in a file encrypted by `systemd-creds --user`,
// with the host key and the TPM2 when there is one: a copy of the file
// opens nothing on another machine.
type SystemdCreds struct {
	Runner CommandRunner
	// Path is the credential file, e.g. ~/.local/share/cade/cade.db.key.cred.
	Path string
}

var _ KeyVault = SystemdCreds{}

// SystemdCredsUsable reports whether runner's systemd-creds has --user.
//
//	usable := dbkey.SystemdCredsUsable(ctx, dbkey.ExecRunner{})
func SystemdCredsUsable(ctx context.Context, runner CommandRunner) bool {
	output, err := runner.Run(ctx, nil, systemdCredsProgram, "--version")
	if err != nil {
		return false
	}
	match := systemdVersionPattern.FindSubmatch(output)
	if match == nil {
		return false
	}
	version, err := strconv.Atoi(string(match[1]))
	return err == nil && version >= systemdCredsUserVersion
}

// Load decrypts the credential file; a missing file is ErrNoKey.
func (s SystemdCreds) Load(ctx context.Context) ([]byte, error) {
	if _, err := os.Stat(s.Path); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no credential file %q: %w", s.Path, ErrNoKey)
	}
	return s.Runner.Run(ctx, nil, systemdCredsProgram, "--user", "decrypt", credentialName, s.Path, stdStream)
}

// Save encrypts key into a partial file, owner-only, and renames it over
// Path, so a failure leaves the old credential in place.
func (s SystemdCreds) Save(ctx context.Context, key []byte) error {
	partial := s.Path + partialCredentialSuffix
	if _, err := s.Runner.Run(ctx, key, systemdCredsProgram, "--user", "encrypt", credentialName, stdStream, partial); err != nil {
		return err
	}
	if err := privatefile.Restrict(partial, credentialFileMode); err != nil {
		return fmt.Errorf("restrict credential %q to %o: %w", partial, credentialFileMode, err)
	}
	return os.Rename(partial, s.Path)
}

// Remove deletes the credential file.
func (s SystemdCreds) Remove(context.Context) error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove credential %q: %w", s.Path, err)
	}
	return nil
}

func (s SystemdCreds) Location() string {
	return "systemd-creds --user, in " + s.Path
}
