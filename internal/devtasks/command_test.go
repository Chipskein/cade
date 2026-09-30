package devtasks

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandStringQuotesArgumentsWithSpaces(t *testing.T) {
	command := Command{Name: "go", Args: []string{"build", "-ldflags", "-X a=b -X c=d"}, Env: []string{"K=v"}}
	if got := command.String(); got != "K=v go build -ldflags '-X a=b -X c=d'" {
		t.Errorf("String() = %q", got)
	}
}

func TestExecRunnerOutputTrimsStdout(t *testing.T) {
	got, err := ExecRunner{Dir: t.TempDir()}.Output(Command{Name: "go", Args: []string{"env", "GOOS"}})
	if err != nil || got != "linux" {
		t.Errorf("Output = %q, %v; want linux", got, err)
	}
}

func TestExecRunnerRunEchoesAndWritesToStdout(t *testing.T) {
	var echo, stdout bytes.Buffer
	err := ExecRunner{Dir: t.TempDir(), Echo: &echo}.Run(Command{Name: "go", Args: []string{"env", "GOOS"}, Stdout: &stdout})
	if err != nil || echo.String() != "go env GOOS\n" || stdout.String() != "linux\n" {
		t.Errorf("echo %q, stdout %q, err %v", echo.String(), stdout.String(), err)
	}
}

func TestExecRunnerRunNamesTheFailedCommand(t *testing.T) {
	var echo bytes.Buffer
	err := ExecRunner{Dir: t.TempDir(), Echo: &echo}.Run(Command{Name: "go", Args: []string{"no-such-subcommand"}})
	if err == nil || !strings.Contains(err.Error(), "go no-such-subcommand") {
		t.Errorf("err = %v, want it to name the command", err)
	}
}
