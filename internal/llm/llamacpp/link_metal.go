//go:build darwin && !cuda && !vulkan

package llamacpp

// On Apple, `go tool mage llama` builds third_party/llama.cpp/build with
// Metal and BLAS (Accelerate) on, llama.cpp's defaults there, and the Metal
// shaders embedded in libggml-metal (#53). The backends go before
// ggml-base, which they use; the frameworks and libc++ come from the system.

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/src -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src
// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src/ggml-metal -L${SRCDIR}/../../../third_party/llama.cpp/build/ggml/src/ggml-blas
// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build/tools/mtmd -L${SRCDIR}/../../../third_party/llama.cpp/build/vendor/hash
// #cgo LDFLAGS: -lmtmd -lvendor-hash -lllama -lggml -lggml-cpu -lggml-metal -lggml-blas -lggml-base -lc++
// #cgo LDFLAGS: -framework Foundation -framework Metal -framework MetalKit -framework Accelerate
import "C"

// buildKind tells saved prompt states of each build apart: their numbers
// differ slightly.
const buildKind = "metal"
