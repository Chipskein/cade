//go:build !cuda && !vulkan && !darwin

package llamacpp

// Static CPU build produced by `go tool mage llama` (third_party/llama.cpp/build).
// llama.cpp is C++: its runtime is libstdc++ with gcc on Linux and libc++
// with FreeBSD's clang (#101). With MinGW on Windows the ggml archives lack
// the lib prefix (ggml.a), so they are named whole, and the link is static:
// otherwise cade.exe needs MinGW's libstdc++ and winpthread DLLs, which a
// Windows machine does not have (#54).

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/src -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src
// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/tools/mtmd -L${SRCDIR}/../../../third_party/llama.cpp/build/vendor/hash
// #cgo !windows LDFLAGS: -lmtmd -lvendor-hash -lllama -lggml -lggml-cpu -lggml-base -lm -lpthread
// #cgo linux LDFLAGS: -lstdc++
// #cgo freebsd LDFLAGS: -lc++
// #cgo windows LDFLAGS: -lmtmd -lvendor-hash -lllama -l:ggml.a -l:ggml-cpu.a -l:ggml-base.a -lstdc++ -static
import "C"

// buildKind tells saved prompt states of CPU and CUDA builds apart: their
// numbers differ slightly.
const buildKind = "cpu"
