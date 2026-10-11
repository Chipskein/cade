//go:build vulkan

package llamacpp

// Vulkan build produced by `go tool mage llamaVulkan` (third_party/llama.cpp/build-vulkan),
// for GPUs without CUDA: NVIDIA on FreeBSD, AMD, Intel (#101). ggml-vulkan
// is static with its shaders inside; only the Vulkan loader is dynamic.

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build-vulkan/src -L${SRCDIR}/../../../third_party/llama.cpp/build-vulkan/ggml/src -L${SRCDIR}/../../../third_party/llama.cpp/build-vulkan/ggml/src/ggml-vulkan
// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build-vulkan/tools/mtmd -L${SRCDIR}/../../../third_party/llama.cpp/build-vulkan/vendor/hash
// #cgo LDFLAGS: -lmtmd -lvendor-hash -lllama -lggml -lggml-cpu -lggml-vulkan -lggml-base -lvulkan -lm -lpthread
// #cgo linux LDFLAGS: -lstdc++
// #cgo freebsd LDFLAGS: -lc++
import "C"

// buildKind tells saved prompt states of each build apart: their numbers
// differ slightly.
const buildKind = "vulkan"
