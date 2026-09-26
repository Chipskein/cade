//go:build cuda

package llamacpp

// CUDA build produced by `make llama-cuda` (third_party/llama.cpp/build-cuda).
// ggml-cuda is static, but the CUDA runtime libraries are linked dynamically;
// the rpath lets the binary find them without LD_LIBRARY_PATH.

// #cgo LDFLAGS: -L${SRCDIR}/../../../third_party/llama.cpp/build-cuda/src -L${SRCDIR}/../../../third_party/llama.cpp/build-cuda/ggml/src -L${SRCDIR}/../../../third_party/llama.cpp/build-cuda/ggml/src/ggml-cuda
// #cgo LDFLAGS: -lllama -lggml -lggml-cpu -lggml-cuda -lggml-base -lstdc++ -lm -lpthread
// #cgo LDFLAGS: -L/opt/cuda/lib64 -Wl,-rpath,/opt/cuda/lib64 -lcudart -lcublas -lcublasLt -lcuda
import "C"
