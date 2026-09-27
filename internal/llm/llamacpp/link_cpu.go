//go:build !cuda

package llamacpp

// Static CPU build produced by `make llama` (third_party/llama.cpp/build).

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/src -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src
// #cgo LDFLAGS: -lllama -lggml -lggml-cpu -lggml-base -lstdc++ -lm -lpthread
import "C"

// buildKind tells saved prompt states of CPU and CUDA builds apart: their
// numbers differ slightly.
const buildKind = "cpu"
