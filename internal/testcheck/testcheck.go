// Package testcheck holds assertions shared by tests in several packages.
package testcheck

import "testing"

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
