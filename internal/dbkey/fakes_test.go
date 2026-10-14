package dbkey

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
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
