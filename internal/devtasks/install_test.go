package devtasks

import (
	"os"
	"path/filepath"
	"testing"
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
	if err != nil || info.Mode().Perm() != executableMode || len(world.runner.Commands) != 0 {
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
