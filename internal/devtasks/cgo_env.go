package devtasks

import "slices"

// FreeBSD installs packages under /usr/local, which its clang does not
// search; sqlite-vec includes the system sqlite3.h (pkg install sqlite3),
// so every cgo build there needs the prefix (#101). Other systems find
// their headers where the compiler looks.
const (
	EnvCgoCFlags  EnvVar = "CGO_CFLAGS"
	EnvCgoLDFlags EnvVar = "CGO_LDFLAGS"
)

var localPrefixFlags = map[string]map[EnvVar]string{
	"freebsd": {EnvCgoCFlags: "-I/usr/local/include", EnvCgoLDFlags: "-L/usr/local/lib"},
}

// cgoTools are the commands that compile cgo, so they get the flags.
var cgoTools = []string{"go", "golangci-lint"}

// cgoEnv is the KEY=VALUE pairs goos needs, after what env already sets.
//
//	cgoEnv("freebsd", env) // ["CGO_CFLAGS=-I/usr/local/include", "CGO_LDFLAGS=-L/usr/local/lib"]
func cgoEnv(goos string, env Environment) []string {
	var pairs []string
	for _, name := range []EnvVar{EnvCgoCFlags, EnvCgoLDFlags} {
		flag, needed := localPrefixFlags[goos][name]
		if !needed {
			continue
		}
		if current, set := env.LookupEnv(name); set && current != "" {
			flag = current + " " + flag
		}
		pairs = append(pairs, string(name)+"="+flag)
	}
	return pairs
}

// cgoRunner adds env to the commands that compile cgo.
type cgoRunner struct {
	CommandRunner
	env []string
}

// withCgoEnv wraps runner when goos needs flags; otherwise it is runner.
func withCgoEnv(runner CommandRunner, goos string, env Environment) CommandRunner {
	pairs := cgoEnv(goos, env)
	if len(pairs) == 0 {
		return runner
	}
	return cgoRunner{CommandRunner: runner, env: pairs}
}

func (r cgoRunner) Run(command Command) error {
	return r.CommandRunner.Run(r.with(command))
}

func (r cgoRunner) Output(command Command) (string, error) {
	return r.CommandRunner.Output(r.with(command))
}

func (r cgoRunner) with(command Command) Command {
	if slices.Contains(cgoTools, command.Name) {
		command.Env = append(slices.Clone(r.env), command.Env...)
	}
	return command
}
