package procctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// TestMain lets StartDetached be tested by starting this test binary,
// the executable os.Executable finds, as the detached process.
func TestMain(m *testing.M) {
	if os.Getenv(detachedHelperVariable) == "1" {
		os.Stdout.WriteString("pid " + strconv.Itoa(os.Getpid()) + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	sleeper := exec.Command("ping", "-n", "30", "127.0.0.1")
	testcheck.NoError(t, sleeper.Start())
	t.Cleanup(func() { sleeper.Process.Kill() })
	return sleeper
}

func TestAliveFindsThisProcess(t *testing.T) {
	if !(System{}).Alive(os.Getpid()) || (System{}).Alive(0) {
		t.Fatal("expected this process alive and pid 0 not")
	}
}

func TestCallsReportMissingProcess(t *testing.T) {
	missing := 1 << 30
	for action, call := range map[string]func(int) error{"stop": System{}.Terminate, "pause": System{}.Pause, "continue": System{}.Continue} {
		if err := call(missing); err == nil || !strings.Contains(err.Error(), action+" process 1073741824") {
			t.Fatalf("expected %q with the pid, got %v", action, err)
		}
	}
}

func TestTerminateStopsAPausedProcess(t *testing.T) {
	sleeper := startSleeper(t)
	pid := sleeper.Process.Pid
	testcheck.NoError(t, System{}.Pause(pid))
	testcheck.NoError(t, System{}.Continue(pid))
	testcheck.NoError(t, System{}.Pause(pid))
	testcheck.NoError(t, System{}.Terminate(pid))
	sleeper.Wait()
	if (System{}).Alive(pid) {
		t.Fatal("expected the paused process ended")
	}
}

func TestStartDetachedWritesItsLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ingest.log")
	pid, err := System{}.StartDetached(DetachedCommand{Env: []string{detachedHelperVariable + "=1"}, LogPath: logPath})
	testcheck.NoError(t, err)
	if logged := waitForLog(t, logPath); logged != "pid "+strconv.Itoa(pid)+"\n" {
		t.Fatalf("expected the child's pid %d in the log, got %q", pid, logged)
	}
}

func TestLowerPrioritySucceeds(t *testing.T) {
	if err := (System{}).LowerPriority(); err != nil {
		t.Fatalf("expected the priority lowered, got %v", err)
	}
}
