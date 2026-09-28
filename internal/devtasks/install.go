package devtasks

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	executableMode fs.FileMode = 0o755
	documentMode   fs.FileMode = 0o644
	directoryMode  fs.FileMode = 0o755
	installedName              = "bin/cade"
)

// cleanedPaths are the build outputs Clean removes; the llama.cpp checkout
// stays, so the next build does not clone it again.
var cleanedPaths = []string{binDir, distDir, coverageProfile, cpuBuildDir, cudaBuildDir}

// Install copies whichever binary is in bin/ (CPU or CUDA) to
// DESTDIR/PREFIX/bin; it builds the CPU one if none exists, so `cuda install`
// keeps the GPU build.
func (t *Tasks) Install() error {
	if !fileExists(t.path(binaryPath)) {
		if err := t.Build(); err != nil {
			return err
		}
	}
	return copyFile(t.path(binaryPath), t.installedPath(), executableMode)
}

// Uninstall removes the installed binary.
func (t *Tasks) Uninstall() error {
	err := os.Remove(t.installedPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Clean removes the build outputs.
func (t *Tasks) Clean() error {
	for _, relative := range cleanedPaths {
		if err := os.RemoveAll(t.path(relative)); err != nil {
			return err
		}
	}
	return nil
}

// installedPath joins DESTDIR and PREFIX by concatenation, as make did:
// DESTDIR is a staging root prepended to an absolute PREFIX.
func (t *Tasks) installedPath() string {
	return filepath.Join(t.settings.DestDir+t.settings.Prefix, installedName)
}

func copyFile(source, destination string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), directoryMode); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(destination, mode)
}
