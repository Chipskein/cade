package procctl

import (
	"os"
	"strings"
	"testing"
	"time"
)

// detachedHelperVariable makes the test binary act as the detached child.
const detachedHelperVariable = "CADE_PROCCTL_HELPER"

// waitForLog polls, since a released child cannot be waited for.
func waitForLog(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.HasSuffix(string(raw), "\n") {
			return string(raw)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the detached process wrote nothing to %s in 10s", path)
	return ""
}
