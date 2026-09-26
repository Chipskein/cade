package gitsource

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CommandRunner runs an external program and returns its stdout. It exists so
// tests can replace the git binary.
type CommandRunner interface {
	Run(ctx context.Context, dir string, program string, args ...string) ([]byte, error)
}

// ExecRunner runs programs with os/exec.
type ExecRunner struct{}

// Run executes program in dir, including stderr in the error on failure.
func (ExecRunner) Run(ctx context.Context, dir string, program string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = dir
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s %v in %q: %w: %s", program, args, dir, err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}
