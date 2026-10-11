//go:build darwin

package procctl

import "testing"

func TestLowerPriorityThrottlesDiskIOOnMacOS(t *testing.T) {
	if err := (System{}).LowerPriority(); err != nil {
		t.Fatalf("expected the priority lowered, got %v", err)
	}
	if !diskIOThrottled() {
		t.Fatal("expected the disk I/O throttled")
	}
}
