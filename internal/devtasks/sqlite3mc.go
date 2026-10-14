package devtasks

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The SQLite3 Multiple Ciphers amalgamation replaces the SQLite one bundled
// with go-sqlite3, so the database can be encrypted (#111, decided in #57).
// go.mod replaces the driver with sqlite3mcDir, which EnsureSQLite3MC
// builds from the pinned driver and the pinned amalgamation. Without a key
// it behaves as the SQLite of the same version, so the readers of other
// SQLite files (browser history, msgstore.db) do not change.
const (
	// GoSQLite3Version must match the require line of go.mod.
	GoSQLite3Version = "v1.14.52"
	SQLite3MCVersion = "2.5.1"
	// sqlite3mcSQLiteVersion is the SQLite that GoSQLite3Version bundles,
	// so the swap changes nothing but the ciphers.
	sqlite3mcSQLiteVersion = "3.53.4"
	sqlite3mcReleases      = "https://github.com/utelle/SQLite3MultipleCiphers/releases/download/v"
	goSQLite3Module        = "github.com/mattn/go-sqlite3"
	sqlite3mcDir           = "third_party/go-sqlite3mc"
	// sqlite3mcStampFile is written last and holds sqlite3mcStamp, so an
	// interrupted build or a version bump rebuilds the directory.
	sqlite3mcStampFile = sqlite3mcDir + "/.cade-stamp"
	sqlite3mcStamp     = GoSQLite3Version + "+sqlite3mc-" + SQLite3MCVersion
)

// pinnedArchive is a download checked against its SHA-256 before use.
type pinnedArchive struct {
	url, sha256 string
}

var sqlite3mcArchive = pinnedArchive{
	url: sqlite3mcReleases + SQLite3MCVersion + "/sqlite3mc-" + SQLite3MCVersion + "-sqlite-" + sqlite3mcSQLiteVersion +
		"-amalgamation.zip",
	sha256: "4125f8ff275ea953dabb3289331b20a0e76d4fc060f57148f4a5df3bf3b0d5e0",
}

// amalgamationFiles maps each file of the archive to the go-sqlite3 file
// it replaces.
var amalgamationFiles = map[string]string{
	"sqlite3mc_amalgamation.c": "sqlite3-binding.c",
	"sqlite3mc_amalgamation.h": "sqlite3-binding.h",
}

// go-sqlite3 compiles its bundled amalgamation only without the libsqlite3
// tag; the replacement keeps that guard.
const (
	libsqlite3GuardOpen  = "#ifndef USE_LIBSQLITE3\n"
	libsqlite3GuardClose = "\n#endif\n"
)

// driverExtraFiles are copied besides the driver's non-test Go, C and
// header files.
var driverExtraFiles = map[string]bool{"go.mod": true, "LICENSE": true}

// EnsureSQLite3MC builds sqlite3mcDir unless it is current.
func (t *Tasks) EnsureSQLite3MC() error {
	return t.buildSQLite3MC(sqlite3mcArchive)
}

func (t *Tasks) buildSQLite3MC(archive pinnedArchive) error {
	if stamp, err := os.ReadFile(t.path(sqlite3mcStampFile)); err == nil && string(stamp) == sqlite3mcStamp {
		return nil
	}
	source, err := t.driverSourceDir()
	if err != nil {
		return err
	}
	amalgamation, err := t.fetchPinned(archive)
	if err != nil {
		return err
	}
	if err := t.writeDriver(source, amalgamation); err != nil {
		return err
	}
	return os.WriteFile(t.path(sqlite3mcStampFile), []byte(sqlite3mcStamp), 0o644)
}

// moduleDownload is the part of `go mod download -json` this task reads.
type moduleDownload struct {
	Dir, Error string
}

// driverSourceDir downloads the pinned driver into the module cache and
// returns where it is; the go.mod replace does not stop that download.
func (t *Tasks) driverSourceDir() (string, error) {
	module := goSQLite3Module + "@" + GoSQLite3Version
	output, err := t.runner.Output(Command{Name: "go", Args: []string{"mod", "download", "-json", module}})
	if err != nil {
		return "", fmt.Errorf("download %s: %w", module, err)
	}
	var download moduleDownload
	if err := json.Unmarshal([]byte(output), &download); err != nil || download.Dir == "" {
		return "", fmt.Errorf("go mod download -json %s: got %q, want a JSON object with Dir (%v %s)", module, output, err, download.Error)
	}
	return download.Dir, nil
}

// fetchPinned downloads archive into memory and checks its SHA-256.
func (t *Tasks) fetchPinned(archive pinnedArchive) (*zip.Reader, error) {
	fmt.Fprintf(t.progress, "downloading %s\n", archive.url)
	var body bytes.Buffer
	if err := t.fetcher.Fetch(archive.url, &body); err != nil {
		return nil, fmt.Errorf("download %s: %w", archive.url, err)
	}
	sum := sha256.Sum256(body.Bytes())
	if got := hex.EncodeToString(sum[:]); got != archive.sha256 {
		return nil, fmt.Errorf("download %s: got SHA-256 %s, want %s", archive.url, got, archive.sha256)
	}
	return zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
}

// writeDriver replaces sqlite3mcDir with the driver from source and the
// amalgamation in place of its SQLite.
func (t *Tasks) writeDriver(source string, amalgamation *zip.Reader) error {
	target := t.path(sqlite3mcDir)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := copyDriverFiles(source, target); err != nil {
		return err
	}
	for archived, replaced := range amalgamationFiles {
		if err := writeGuarded(amalgamation, archived, filepath.Join(target, replaced)); err != nil {
			return err
		}
	}
	return nil
}

// copyDriverFiles copies the top level of the driver: its examples and
// tests are not part of the build.
func copyDriverFiles(source, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isDriverFile(entry.Name()) {
			continue
		}
		if err := copyWritable(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func isDriverFile(name string) bool {
	if driverExtraFiles[name] {
		return true
	}
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	return strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".c") || strings.HasSuffix(name, ".h")
}

// copyWritable copies path to destination as 0644: the module cache is
// read-only, and a rebuild has to remove the copy.
func copyWritable(path, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := copyInto(file, path); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// writeGuarded writes the archived file to destination inside the
// libsqlite3 guard.
func writeGuarded(amalgamation *zip.Reader, archived, destination string) error {
	content, err := amalgamation.Open(archived)
	if err != nil {
		return fmt.Errorf("sqlite3mc archive: %w; want it to contain %s", err, archived)
	}
	defer content.Close()
	var guarded bytes.Buffer
	guarded.WriteString(libsqlite3GuardOpen)
	if _, err := io.Copy(&guarded, content); err != nil {
		return err
	}
	guarded.WriteString(libsqlite3GuardClose)
	return os.WriteFile(destination, guarded.Bytes(), 0o644)
}
