package devtasks

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// distFiles are the licenses and docs a user needs next to the binary.
var distFiles = []string{
	"LICENSE", "THIRD_PARTY_NOTICES.md", "README.md", "README.pt-BR.md", "PRIVACY.md", "PRIVACY.pt-BR.md",
	"CHANGELOG.md", "CHANGELOG.pt-BR.md", "config.example.json",
}

const (
	archiveSuffix  = ".tar.gz"
	checksumSuffix = ".sha256"
)

// Dist builds the release archive, dist/cade-<version>-linux-<arch>-cpu.tar.gz,
// and its SHA-256. The release workflow runs it with LLAMA_NATIVE=OFF so it
// runs on any x86-64 CPU with AVX2; CUDA stays a local build (it ties the
// binary to a driver and GPU architecture).
func (t *Tasks) Dist() error {
	if err := t.Build(); err != nil {
		return err
	}
	name, err := t.distName()
	if err != nil {
		return err
	}
	if err := t.stageDist(name); err != nil {
		return err
	}
	archive := t.path(filepath.Join(distDir, name+archiveSuffix))
	if err := writeTarGz(archive, t.path(distDir), name); err != nil {
		return err
	}
	return writeChecksum(archive)
}

func (t *Tasks) distName() (string, error) {
	arch, err := t.runner.Output(Command{Name: "go", Args: []string{"env", "GOARCH"}})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("cade-%s-linux-%s-cpu", t.resolveBuildStamp().version, arch), nil
}

func (t *Tasks) stageDist(name string) error {
	staging := t.path(filepath.Join(distDir, name))
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := copyFile(t.path(binaryPath), filepath.Join(staging, filepath.Base(binaryPath)), executableMode); err != nil {
		return err
	}
	for _, file := range distFiles {
		if err := copyFile(t.path(file), filepath.Join(staging, file), documentMode); err != nil {
			return err
		}
	}
	return nil
}

// writeTarGz archives baseDir/entry with paths starting at entry.
func writeTarGz(archive, baseDir, entry string) error {
	file, err := os.Create(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed := gzip.NewWriter(file)
	tarball := tar.NewWriter(compressed)
	if err := addTreeToTar(tarball, baseDir, entry); err != nil {
		return err
	}
	if err := tarball.Close(); err != nil {
		return err
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	return file.Close()
}

func addTreeToTar(tarball *tar.Writer, baseDir, entry string) error {
	return filepath.WalkDir(filepath.Join(baseDir, entry), func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return addToTar(tarball, baseDir, path)
	})
}

func addToTar(tarball *tar.Writer, baseDir, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	if header.Name, err = filepath.Rel(baseDir, path); err != nil {
		return err
	}
	if err := tarball.WriteHeader(header); err != nil || info.IsDir() {
		return err
	}
	return copyInto(tarball, path)
}

func copyInto(destination io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}

// writeChecksum writes archive.sha256 in sha256sum's format, so
// `sha256sum -c` checks it from the directory the archive is in.
func writeChecksum(archive string) error {
	hash := sha256.New()
	if err := copyInto(hash, archive); err != nil {
		return err
	}
	line := fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash.Sum(nil)), filepath.Base(archive))
	return os.WriteFile(archive+checksumSuffix, []byte(line), documentMode)
}
