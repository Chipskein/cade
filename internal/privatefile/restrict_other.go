//go:build !windows

package privatefile

import (
	"io/fs"
	"os"
)

// ModesEnforced says Restrict sets the mode here.
const ModesEnforced = true

func restrict(path string, mode fs.FileMode) error {
	return os.Chmod(path, mode)
}
