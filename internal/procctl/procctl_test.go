//go:build linux

package procctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/testcheck"
)

// detachedHelperVariable makes the test binary act as the detached child.
const detachedHelperVariable = "CADE_PROCCTL_HELPER"

// TestMain lets StartDetached be tested by starting this test binary,
// the executable os.Executable finds, as the detached process.
func TestMain(m *testing.M) {
	if os.Getenv(detachedHelperVariable) == "1" {
		sessionID, _, _ := syscall.RawSyscall(syscall.SYS_GETSID, 0, 0, 0)
		os.Stdout.WriteString("session " + strconv.Itoa(int(sessionID)) + " pid " + strconv.Itoa(os.Getpid()) + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestAliveFindsThisProcess(t *testing.T) {
	if !(System{}).Alive(os.Getpid()) || (System{}).Alive(0) {
		t.Fatal("expected this process alive and pid 0 not")
	}
}

func TestSignalsReportMissingProcess(t *testing.T) {
	missing := 1 << 30
	for action, send := range map[string]func(int) error{"stop": System{}.Terminate, "pause": System{}.Pause, "continue": System{}.Continue} {
		if err := send(missing); err == nil || !strings.Contains(err.Error(), action+" process 1073741824") {
			t.Fatalf("expected %q with the pid, got %v", action, err)
		}
	}
}

func TestPauseFreezesAndContinueWakes(t *testing.T) {
	sleeper := exec.Command("sleep", "30")
	testcheck.NoError(t, sleeper.Start())
	defer func() { testcheck.NoError(t, sleeper.Process.Kill()) }()
	pid := sleeper.Process.Pid
	testcheck.NoError(t, System{}.Pause(pid))
	waitForProcessState(t, pid, "T")
	testcheck.NoError(t, System{}.Continue(pid))
	waitForProcessState(t, pid, "S")
}

func TestTerminateStopsAPausedProcess(t *testing.T) {
	sleeper := exec.Command("sleep", "30")
	testcheck.NoError(t, sleeper.Start())
	testcheck.NoError(t, System{}.Pause(sleeper.Process.Pid))
	testcheck.NoError(t, System{}.Terminate(sleeper.Process.Pid))
	if err := sleeper.Wait(); err == nil || !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected the paused process terminated, got %v", err)
	}
}

// waitForProcessState polls the state letter of /proc/PID/stat: "T" is
// stopped by a signal, "S" sleeping.
func waitForProcessState(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		fields := strings.Fields(string(raw))
		if err == nil && len(fields) > 2 && fields[2] == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d never reached state %q", pid, want)
}

func TestStartDetachedRunsInItsOwnSessionWithLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ingest.log")
	pid, err := System{}.StartDetached(DetachedCommand{Env: []string{detachedHelperVariable + "=1"}, LogPath: logPath})
	testcheck.NoError(t, err)
	logged := waitForLog(t, logPath)
	if logged != "session "+strconv.Itoa(pid)+" pid "+strconv.Itoa(pid)+"\n" {
		t.Fatalf("expected the child to lead its own session (pid %d), got %q", pid, logged)
	}
}

func TestStartDetachedReportsUnwritableLog(t *testing.T) {
	_, err := System{}.StartDetached(DetachedCommand{LogPath: filepath.Join(t.TempDir(), "missing", "ingest.log")})
	if err == nil || !strings.Contains(err.Error(), "open ingest log") {
		t.Fatalf("expected the log path reported, got %v", err)
	}
}

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

func TestLowerPriorityRenicesThisProcess(t *testing.T) {
	if err := (System{}).LowerPriority(); err != nil {
		t.Fatalf("expected the priority lowered, got %v", err)
	}
	nice, err := syscall.Getpriority(syscall.PRIO_PROCESS, 0)
	testcheck.NoError(t, err)
	// Go returns the raw syscall value on Linux: 20 - nice.
	if nice != 20-lowestNice {
		t.Fatalf("expected nice %d, got raw priority %d", lowestNice, nice)
	}
}
