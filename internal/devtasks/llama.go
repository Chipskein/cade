package devtasks

import (
	"fmt"
	"path"
)

// LlamaVariant is which llama.cpp build a target links against.
type LlamaVariant int

const (
	LlamaCPU LlamaVariant = iota
	LlamaCUDA
)

const (
	cpuBuildDir  = llamaDir + "/build"
	cudaBuildDir = llamaDir + "/build-cuda"
	// libmtmd.a is the last library built, so a build directory from before
	// mtmd was added is reconfigured and completed.
	lastLlamaLibrary = "tools/mtmd/libmtmd.a"
)

// llamaCMakeFlags build only the llama and mtmd (vision) libraries: no
// tools, server or downloader (LLAMA_CURL off), so nothing in the binary can
// reach the network (RNF1). Without subprocesses mtmd has no video, which
// would run ffmpeg.
var llamaCMakeFlags = []string{
	"-G", "Ninja", "-DCMAKE_BUILD_TYPE=Release", "-DBUILD_SHARED_LIBS=OFF",
	"-DCMAKE_POSITION_INDEPENDENT_CODE=ON", "-DGGML_OPENMP=OFF", "-DLLAMA_CURL=OFF",
	"-DLLAMA_BUILD_TESTS=OFF", "-DLLAMA_BUILD_EXAMPLES=OFF", "-DLLAMA_BUILD_TOOLS=OFF", "-DLLAMA_BUILD_SERVER=OFF",
	"-DLLAMA_BUILD_COMMON=OFF", "-DLLAMA_BUILD_MTMD=ON", "-DLLAMA_SUBPROCESS=OFF", "-DMTMD_VIDEO=OFF",
}

var llamaTargets = []string{"--target", "llama", "mtmd"}

func (v LlamaVariant) buildDir() string {
	if v == LlamaCUDA {
		return cudaBuildDir
	}
	return cpuBuildDir
}

// EnsureLlama clones the pinned llama.cpp and builds variant, unless its
// libraries are already built.
func (t *Tasks) EnsureLlama(variant LlamaVariant) error {
	if fileExists(t.path(path.Join(variant.buildDir(), lastLlamaLibrary))) {
		return nil
	}
	if err := t.cloneLlama(); err != nil {
		return err
	}
	if err := t.requireToolkit(variant); err != nil {
		return err
	}
	if err := t.runner.Run(Command{Name: "cmake", Args: t.llamaConfigureArgs(variant)}); err != nil {
		return err
	}
	return t.runner.Run(Command{Name: "cmake", Args: append([]string{"--build", variant.buildDir()}, llamaTargets...)})
}

func (t *Tasks) cloneLlama() error {
	if fileExists(t.path(llamaDir)) {
		return nil
	}
	return t.runner.Run(Command{Name: "git", Args: []string{"clone", "--depth", "1", "--branch", LlamaTag, llamaRepoURL, llamaDir}})
}

func (t *Tasks) requireToolkit(variant LlamaVariant) error {
	if variant != LlamaCUDA || fileExists(t.settings.NvccPath()) {
		return nil
	}
	return fmt.Errorf("nvcc não encontrado em %q; instale o CUDA Toolkit (pacman -S cuda) ou aponte CUDA_HOME para ele", t.settings.NvccPath())
}

func (t *Tasks) llamaConfigureArgs(variant LlamaVariant) []string {
	args := append([]string{"-S", llamaDir, "-B", variant.buildDir()}, llamaCMakeFlags...)
	args = append(args, "-DGGML_NATIVE="+t.settings.LlamaNative)
	if variant == LlamaCUDA {
		args = append(args, t.cudaConfigureArgs()...)
	}
	return args
}

// cudaConfigureArgs passes NVCC_CCBIN because nvcc rejects host compilers
// newer than it supports; Arch's cuda package exports a compatible one there
// (/etc/profile.d/cuda.sh).
func (t *Tasks) cudaConfigureArgs() []string {
	args := []string{"-DGGML_CUDA=ON", "-DCMAKE_CUDA_ARCHITECTURES=" + t.settings.CudaArch,
		"-DCMAKE_CUDA_COMPILER=" + t.settings.NvccPath()}
	if t.settings.NvccCCBin != "" {
		args = append(args, "-DCMAKE_CUDA_HOST_COMPILER="+t.settings.NvccCCBin)
	}
	return args
}
