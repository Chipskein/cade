package cli

import (
	"context"
	"fmt"
)

// runVersion prints the version, commit, date and build type: `cade
// version` or `cade --version`.
func runVersion(_ context.Context, env commandEnv, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf(env.language.pick("cade version não recebe argumentos, recebido %q", "cade version takes no arguments, got %q"), args)
	}
	fmt.Fprintln(env.stdout, env.toolkit.Build)
	return nil
}
