package devtasks

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const distTestName = "cade-v9.9.9-linux-amd64-cpu"

func TestDistArchivesTheBinaryAndDocs(t *testing.T) {
	world := newTestWorld(t)
	world.env[EnvVersion] = "v9.9.9"
	world.runner.Outputs["go env GOARCH"] = "amd64"
	world.writeFile(t, binaryPath, "binary")
	for _, file := range distFiles {
		world.writeFile(t, file, file)
	}
	if err := world.tasks().Dist(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(world.root, distDir, distTestName+archiveSuffix)
	want := append([]string{distTestName, distTestName + "/cade"}, prefixed(distTestName+"/", distFiles)...)
	if got := tarNames(t, archive); !slices.Equal(sorted(got), sorted(want)) {
		t.Errorf("archive holds %q, want %q", got, want)
	}
	assertChecksum(t, archive)
}

func prefixed(prefix string, names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, prefix+name)
	}
	return out
}

func sorted(names []string) []string {
	return slices.Sorted(slices.Values(names))
}

func tarNames(t *testing.T, archive string) []string {
	t.Helper()
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for reader := tar.NewReader(compressed); ; {
		header, err := reader.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
}

func assertChecksum(t *testing.T, archive string) {
	t.Helper()
	body, _ := os.ReadFile(archive)
	sum := sha256.Sum256(body)
	line, err := os.ReadFile(archive + checksumSuffix)
	if err != nil || string(line) != hex.EncodeToString(sum[:])+"  "+filepath.Base(archive)+"\n" {
		t.Errorf("checksum file = %q, %v", line, err)
	}
	if !strings.HasSuffix(archive, archiveSuffix) {
		t.Errorf("archive %q lacks %s", archive, archiveSuffix)
	}
}

// Regression: the archive has no assets/, so a relative image path in a
// doc it ships showed a broken image.
func TestDistDocsLoadImagesFromTheRepository(t *testing.T) {
	for _, file := range distFiles {
		if !strings.HasSuffix(file, ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `src="assets/`) || (strings.Contains(string(raw), "<img") && !strings.Contains(string(raw), distImageURL)) {
			t.Errorf("%s: images must load from %s, not a relative path", file, distImageURL)
		}
	}
}
