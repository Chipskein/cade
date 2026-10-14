package dbkey

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
)

// sealedPrefix marks what FakeSystemdCredsRunner "encrypts", so a test can
// tell a sealed file from the raw key.
const sealedPrefix = "sealed:"

// FakeSystemdCredsRunner behaves as systemd-creds: encrypt writes stdin,
// sealed, to its output argument and decrypt reads it back. It answers
// --version with Version, records each call and fails those whose line
// contains FailOn.
type FakeSystemdCredsRunner struct {
	Version string
	FailOn  string
	Calls   []string
}

func (f *FakeSystemdCredsRunner) Run(_ context.Context, stdin []byte, program string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{program}, args...), " ")
	f.Calls = append(f.Calls, line)
	if f.FailOn != "" && strings.Contains(line, f.FailOn) {
		return nil, errors.New("exit status 1")
	}
	switch {
	case slices.Contains(args, "--version"):
		return []byte(f.Version), nil
	case slices.Contains(args, "encrypt"):
		return nil, os.WriteFile(args[len(args)-1], append([]byte(sealedPrefix), stdin...), 0o644)
	case slices.Contains(args, "decrypt"):
		sealed, err := os.ReadFile(args[len(args)-2])
		return bytes.TrimPrefix(sealed, []byte(sealedPrefix)), err
	}
	return nil, errors.New("unexpected call " + line)
}

// fakeExitError is a tool's exit status, as exec.ExitError reports it.
type fakeExitError struct {
	code int
}

func (e fakeExitError) Error() string { return "exit status " + strconv.Itoa(e.code) }

func (e fakeExitError) ExitCode() int { return e.code }

// FakeSecurityRunner behaves as macOS's security on one Keychain item: the
// interactive add stores the -w value, find prints it, delete removes it,
// and a missing item exits 44. It records each call with its stdin.
type FakeSecurityRunner struct {
	Item   string
	Stored bool
	Calls  []string
}

func (f *FakeSecurityRunner) Run(_ context.Context, stdin []byte, program string, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, strings.Join(append([]string{program}, args...), " ")+" <"+string(stdin))
	switch {
	case slices.Equal(args, []string{"-i"}):
		fields := strings.Fields(string(stdin))
		f.Item, f.Stored = fields[slices.Index(fields, "-w")+1], true
		return nil, nil
	case !f.Stored:
		return nil, fakeExitError{code: keychainItemNotFound}
	case args[0] == "find-generic-password":
		return []byte(f.Item + "\n"), nil
	case args[0] == "delete-generic-password":
		f.Item, f.Stored = "", false
		return nil, nil
	}
	return nil, errors.New("unexpected call " + strings.Join(args, " "))
}
