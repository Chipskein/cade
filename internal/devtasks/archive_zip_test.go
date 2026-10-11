package devtasks

import (
	"archive/zip"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// On Windows the release is a zip holding cade.exe (#54).
func TestDistZipsCadeExeOnWindows(t *testing.T) {
	world := newTestWorld(t)
	world.goos = "windows"
	world.env[EnvVersion] = "v9.9.9"
	world.runner.Outputs["go env GOOS"] = "windows"
	world.runner.Outputs["go env GOARCH"] = "amd64"
	world.writeFile(t, "bin/cade.exe", "binary")
	for _, file := range distFiles {
		world.writeFile(t, file, file)
	}
	testcheck.NoError(t, world.tasks().Dist())
	name := "cade-v9.9.9-windows-amd64-cpu"
	archive := filepath.Join(world.root, distDir, name+zipSuffix)
	want := append([]string{name + "/", name + "/cade.exe"}, prefixed(name+"/", distFiles)...)
	if got := zipNames(t, archive); !slices.Equal(sorted(got), sorted(want)) {
		t.Errorf("archive holds %q, want %q", got, want)
	}
	assertChecksum(t, archive)
}

func zipNames(t *testing.T, archive string) []string {
	t.Helper()
	reader, err := zip.OpenReader(archive)
	testcheck.NoError(t, err)
	defer reader.Close()
	names := make([]string, len(reader.File))
	for i, file := range reader.File {
		names[i] = file.Name
	}
	return names
}
