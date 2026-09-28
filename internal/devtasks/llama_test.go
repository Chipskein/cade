package devtasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureLlamaSkipsABuiltLibrary(t *testing.T) {
	world := newTestWorld(t)
	if err := world.tasks().EnsureLlama(LlamaCPU); err != nil || len(world.runner.Commands) != 0 {
		t.Errorf("err %v, commands %q; want nothing run", err, world.runner.Lines())
	}
}

func TestEnsureLlamaClonesAndBuildsCPU(t *testing.T) {
	world := newTestWorld(t)
	if err := os.RemoveAll(filepath.Join(world.root, llamaDir)); err != nil {
		t.Fatal(err)
	}
	if err := world.tasks().EnsureLlama(LlamaCPU); err != nil {
		t.Fatal(err)
	}
	lines := world.runner.Lines()
	if len(lines) != 3 || lines[0] != "git clone --depth 1 --branch b11195 https://github.com/ggml-org/llama.cpp third_party/llama.cpp" {
		t.Fatalf("commands = %q", lines)
	}
	if !strings.HasPrefix(lines[1], "cmake -S third_party/llama.cpp -B third_party/llama.cpp/build -G Ninja") || !strings.HasSuffix(lines[1], "-DGGML_NATIVE=ON") {
		t.Errorf("configure = %q", lines[1])
	}
	if lines[2] != "cmake --build third_party/llama.cpp/build --target llama mtmd" {
		t.Errorf("build = %q", lines[2])
	}
}

func TestEnsureLlamaCUDARequiresNvcc(t *testing.T) {
	world := newTestWorld(t)
	err := world.tasks().EnsureLlama(LlamaCUDA)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(world.root, "cuda", "bin", "nvcc")) {
		t.Errorf("err = %v, want it to name the missing nvcc", err)
	}
}

func TestEnsureLlamaCUDAPassesTheHostCompiler(t *testing.T) {
	world := newTestWorld(t)
	world.writeFile(t, "cuda/bin/nvcc", "")
	world.env[EnvNvccCCBin] = "/usr/bin/g++-14"
	if err := world.tasks().EnsureLlama(LlamaCUDA); err != nil {
		t.Fatal(err)
	}
	configure := world.runner.Lines()[0]
	for _, flag := range []string{"-B third_party/llama.cpp/build-cuda", "-DGGML_CUDA=ON", "-DCMAKE_CUDA_ARCHITECTURES=86", "-DCMAKE_CUDA_HOST_COMPILER=/usr/bin/g++-14"} {
		if !strings.Contains(configure, flag) {
			t.Errorf("configure %q lacks %q", configure, flag)
		}
	}
}
