package dbkey

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner runs a system tool that keeps secrets (systemd-creds,
// security), so tests can replace it.
type CommandRunner interface {
	// Run feeds stdin to program and returns its stdout. The key travels
	// on stdin or stdout, never in the arguments, which other processes
	// can read.
	Run(ctx context.Context, stdin []byte, program string, args ...string) ([]byte, error)
}

// ExecRunner runs programs with os/exec.
type ExecRunner struct{}

// Run executes program, with its stderr in the error on failure.
//
//	output, err := dbkey.ExecRunner{}.Run(ctx, nil, "systemd-creds", "--version")
func (ExecRunner) Run(ctx context.Context, stdin []byte, program string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Stdin = bytes.NewReader(stdin)
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s %s: %w: %s", program, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}
