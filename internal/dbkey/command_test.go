package dbkey

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// helperProcessVariable makes the test binary act as the external tool:
// it echoes stdin back, or fails, so ExecRunner is tested on every system
// without depending on a tool being installed.
const helperProcessVariable = "CADE_DBKEY_HELPER_PROCESS"

func TestMain(m *testing.M) {
	switch os.Getenv(helperProcessVariable) {
	case "echo":
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "fail":
		os.Stderr.WriteString("credential is locked")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestExecRunnerPassesStdinAndReturnsStdout(t *testing.T) {
	t.Setenv(helperProcessVariable, "echo")
	output, err := ExecRunner{}.Run(context.Background(), []byte("the key"), os.Args[0])
	if err != nil || string(output) != "the key" {
		t.Errorf("output = %q, %v; want stdin echoed", output, err)
	}
}

func TestExecRunnerReportsStderr(t *testing.T) {
	t.Setenv(helperProcessVariable, "fail")
	_, err := ExecRunner{}.Run(context.Background(), nil, os.Args[0], "decrypt")
	if err == nil || !strings.Contains(err.Error(), "credential is locked") || !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("err = %v, want the command and its stderr", err)
	}
}
