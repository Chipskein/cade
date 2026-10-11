//go:build windows

package winapi

import (
	"os"
	"os/exec"
	"testing"
)

// startSleeper runs a process that waits, for the calls to act on.
func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	sleeper := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleeper.Process.Kill() })
	return sleeper
}

func TestProcessRunningFindsThisProcessAndNotAnEndedOne(t *testing.T) {
	if !ProcessRunning(os.Getpid()) {
		t.Fatal("expected this process running")
	}
	sleeper := startSleeper(t)
	if err := TerminateProcess(sleeper.Process.Pid); err != nil {
		t.Fatalf("expected the sleeper terminated, got %v", err)
	}
	_ = sleeper.Wait() // ended by the test, so it reports an error
	if ProcessRunning(sleeper.Process.Pid) {
		t.Fatal("expected the terminated sleeper not running")
	}
}

func TestSuspendAndResumeAProcess(t *testing.T) {
	pid := startSleeper(t).Process.Pid
	if err := SuspendProcess(pid); err != nil {
		t.Fatalf("expected the sleeper suspended, got %v", err)
	}
	if err := ResumeProcess(pid); err != nil {
		t.Fatalf("expected the sleeper resumed, got %v", err)
	}
	if !ProcessRunning(pid) {
		t.Fatal("expected the resumed sleeper running")
	}
}

func TestCallsReportAMissingProcess(t *testing.T) {
	missing := 1 << 30
	for name, call := range map[string]func(int) error{"terminate": TerminateProcess, "suspend": SuspendProcess, "resume": ResumeProcess} {
		if err := call(missing); err == nil {
			t.Errorf("%s: expected an error for missing pid %d", name, missing)
		}
	}
}

func TestEnterBackgroundModeSucceeds(t *testing.T) {
	if err := EnterBackgroundMode(); err != nil {
		t.Fatalf("expected background mode, got %v", err)
	}
}
