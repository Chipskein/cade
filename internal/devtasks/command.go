package devtasks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Command is one external program run by a task.
type Command struct {
	Name string
	Args []string
	// Env holds KEY=VALUE pairs added to the inherited environment.
	Env []string
	// Stdout receives the output; nil means the terminal.
	Stdout io.Writer
}

// String is the command line as echoed before it runs, with arguments that
// hold spaces quoted so it can be pasted into a shell.
func (c Command) String() string {
	parts := append(append([]string{}, c.Env...), c.Name)
	for _, arg := range c.Args {
		parts = append(parts, shellQuoted(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuoted(arg string) string {
	if !strings.ContainsAny(arg, " \t") {
		return arg
	}
	return "'" + arg + "'"
}

// CommandRunner runs external programs; ExecRunner is the real one.
type CommandRunner interface {
	Run(command Command) error
	// Output returns the command's stdout without surrounding whitespace.
	Output(command Command) (string, error)
}

// ExecRunner runs commands in Dir, echoing each one to Echo like make did.
type ExecRunner struct {
	Dir  string
	Echo io.Writer
}

// Run runs command with its output on the terminal (or command.Stdout).
func (r ExecRunner) Run(command Command) error {
	fmt.Fprintln(r.Echo, command.String())
	cmd := r.prepare(command)
	cmd.Stdout = os.Stdout
	if command.Stdout != nil {
		cmd.Stdout = command.Stdout
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", command.String(), err)
	}
	return nil
}

// Output runs command without echoing it and returns its stdout.
func (r ExecRunner) Output(command Command) (string, error) {
	var stdout bytes.Buffer
	cmd := r.prepare(command)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", command.String(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (r ExecRunner) prepare(command Command) *exec.Cmd {
	cmd := exec.Command(command.Name, command.Args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), command.Env...)
	cmd.Stderr = os.Stderr
	return cmd
}
