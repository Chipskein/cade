package devtasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/privatefile"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestInstallCopiesTheBuiltBinaryUnderDestDirAndPrefix(t *testing.T) {
	world := newTestWorld(t)
	world.writeFile(t, binaryPath, "cuda binary")
	world.env[EnvDestDir], world.env[EnvPrefix] = filepath.Join(world.root, "stage"), "/usr"
	if err := world.tasks().Install(); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(world.root, "stage/usr/bin/cade")
	info, err := os.Stat(installed)
	if err != nil || (privatefile.ModesEnforced && info.Mode().Perm() != executableMode) || len(world.runner.Commands) != 0 {
		t.Fatalf("installed %v, %v; commands %q (want no build)", info, err, world.runner.Lines())
	}
}

func TestInstallBuildsWhenNoBinaryExists(t *testing.T) {
	world := newTestWorld(t)
	world.env[EnvPrefix] = filepath.Join(world.root, "prefix")
	world.runner.FailOn = "go build"
	if err := world.tasks().Install(); err == nil {
		t.Error("Install succeeded without a binary to copy")
	}
}

func TestUninstallToleratesAMissingBinary(t *testing.T) {
	world := newTestWorld(t)
	world.env[EnvPrefix] = filepath.Join(world.root, "prefix")
	installed := world.writeFile(t, "prefix/bin/cade", "")
	tasks := world.tasks()
	if err := tasks.Uninstall(); err != nil || fileExists(installed) {
		t.Fatalf("first uninstall: %v (still there: %v)", err, fileExists(installed))
	}
	if err := tasks.Uninstall(); err != nil {
		t.Errorf("second uninstall: %v", err)
	}
}

func TestCleanKeepsTheLlamaCheckout(t *testing.T) {
	world := newTestWorld(t)
	world.writeFile(t, binaryPath, "")
	world.writeFile(t, llamaDir+"/CMakeLists.txt", "")
	if err := world.tasks().Clean(); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(world.root, binaryPath)) || fileExists(filepath.Join(world.root, cpuBuildDir)) {
		t.Error("build outputs remain")
	}
	if !fileExists(filepath.Join(world.root, llamaDir, "CMakeLists.txt")) {
		t.Error("the llama.cpp checkout was removed")
	}
}

// Windows runs only files named .exe (#54).
func TestWindowsBuildsAndInstallsCadeExe(t *testing.T) {
	world := newTestWorld(t)
	world.goos = "windows"
	world.env[EnvPrefix] = filepath.Join(world.root, "prefix")
	testcheck.NoError(t, world.tasks().BuildCPU())
	if !strings.Contains(strings.Join(world.runner.Lines(), "\n"), "-o bin/cade.exe") {
		t.Fatalf("expected the build to write bin/cade.exe, got %q", world.runner.Lines())
	}
	world.writeFile(t, "bin/cade.exe", "binary")
	testcheck.NoError(t, world.tasks().Install())
	if !fileExists(filepath.Join(world.root, "prefix", "bin", "cade.exe")) {
		t.Fatal("expected cade.exe installed under the prefix")
	}
}
