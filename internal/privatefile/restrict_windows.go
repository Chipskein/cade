package privatefile

import (
	"errors"
	"io/fs"
	"os"
)

// ModesEnforced says Restrict leaves the mode alone here.
const ModesEnforced = false

// restrict still reports a missing path, as os.Chmod does on unix, so
// callers that skip missing files behave the same.
func restrict(path string, _ fs.FileMode) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
