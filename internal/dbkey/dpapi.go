package dbkey

import (
	"context"
	"fmt"
	"os"
)

// BlobProtector seals data for the current user, as Windows' DPAPI does;
// WindowsDPAPI is the real one.
type BlobProtector interface {
	Protect(data []byte) ([]byte, error)
	Unprotect(blob []byte) ([]byte, error)
}

// DPAPIFile keeps the key in a file sealed by Protector: with DPAPI, only
// the same Windows account opens it, so a copy of the file is useless.
type DPAPIFile struct {
	Protector BlobProtector
	// Path is the sealed file, e.g. %LOCALAPPDATA%\cade\cade.db.key.dpapi.
	Path string
}

var _ KeyVault = DPAPIFile{}

// Load unseals the file; a missing file is ErrNoKey.
func (d DPAPIFile) Load(context.Context) ([]byte, error) {
	if err := missingKeyFile(d.Path); err != nil {
		return nil, err
	}
	blob, err := os.ReadFile(d.Path)
	if err != nil {
		return nil, fmt.Errorf("read key file %q: %w", d.Path, err)
	}
	return d.Protector.Unprotect(blob)
}

// Save seals key into a partial file and renames it over Path.
func (d DPAPIFile) Save(_ context.Context, key []byte) error {
	blob, err := d.Protector.Protect(key)
	if err != nil {
		return err
	}
	partial := d.Path + partialKeyFileSuffix
	if err := os.WriteFile(partial, blob, keyFileMode); err != nil {
		return fmt.Errorf("write key file %q: %w", partial, err)
	}
	return publishKeyFile(partial, d.Path)
}

// Remove deletes the sealed file.
func (d DPAPIFile) Remove(context.Context) error {
	return removeKeyFile(d.Path)
}

func (d DPAPIFile) Location() string {
	return "DPAPI (your Windows account), in " + d.Path
}
