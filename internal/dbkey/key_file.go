package dbkey

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/chipskein/cade/internal/privatefile"
)

// The vaults that keep the key in a file of their own (systemd-creds,
// DPAPI) share these steps; the file holds the key sealed, never in the
// clear.
const (
	keyFileMode          = 0o600
	partialKeyFileSuffix = ".part"
)

// missingKeyFile is ErrNoKey when path does not exist, and nil otherwise.
func missingKeyFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no key file %q: %w", path, ErrNoKey)
	}
	return nil
}

// publishKeyFile makes partial owner-only and renames it over path, so a
// failed save leaves the old file in place.
func publishKeyFile(partial, path string) error {
	if err := privatefile.Restrict(partial, keyFileMode); err != nil {
		return fmt.Errorf("restrict key file %q to %o: %w", partial, keyFileMode, err)
	}
	return os.Rename(partial, path)
}

// removeKeyFile deletes path; a missing file is not an error.
func removeKeyFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove key file %q: %w", path, err)
	}
	return nil
}
