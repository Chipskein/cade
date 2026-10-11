//go:build !cuda && !vulkan

package llamacpp

// Static CPU build produced by `go tool mage llama` (third_party/llama.cpp/build).
// llama.cpp is C++: its runtime is libstdc++ with gcc on Linux and libc++
// with FreeBSD's clang (#101).

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/src -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src
// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/tools/mtmd -L${SRCDIR}/../../../third_party/llama.cpp/build/vendor/hash
// #cgo LDFLAGS: -lmtmd -lvendor-hash -lllama -lggml -lggml-cpu -lggml-base -lm -lpthread
// #cgo linux LDFLAGS: -lstdc++
// #cgo freebsd LDFLAGS: -lc++
import "C"

// buildKind tells saved prompt states of CPU and CUDA builds apart: their
// numbers differ slightly.
const buildKind = "cpu"
