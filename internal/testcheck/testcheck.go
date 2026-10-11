// Package testcheck holds assertions shared by tests in several packages.
package testcheck

import (
	"os"
	"testing"

	"github.com/chipskein/cade/internal/privatefile"
)

// ownerOnlyMode is the mode of cade's private files.
const ownerOnlyMode = 0o600

// NoError stops the test when a setup step fails, so later assertions
// never run against a half-built fixture and report a misleading cause.
//
//	testcheck.NoError(t, os.WriteFile(path, body, 0o600))
func NoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup step failed: %v (expected no error)", err)
	}
}

// OwnerOnly fails the test unless path exists and, on a system with file
// modes, is readable by its owner only; Windows keeps it private through
// the profile's ACL instead (#54).
//
//	testcheck.OwnerOnly(t, "/home/me/.config/cade/config.json")
func OwnerOnly(t testing.TB, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v (expected an owner-only file)", path, err)
	}
	if privatefile.ModesEnforced && info.Mode().Perm() != ownerOnlyMode {
		t.Fatalf("expected %q with mode %o, got %o", path, ownerOnlyMode, info.Mode().Perm())
	}
}
