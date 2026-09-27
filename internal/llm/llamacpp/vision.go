package llamacpp

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/llama.cpp/tools/mtmd
#include <stdlib.h>
#include "mtmd.h"

static void cade_discard_vision_log(enum ggml_log_level level, const char * text, void * user_data) {
	(void)level; (void)text; (void)user_data;
}

static struct mtmd_context * cade_load_projector(const char * path, const struct llama_model * model, bool use_gpu, int threads) {
	mtmd_log_set(cade_discard_vision_log, NULL);
	struct mtmd_context_params params = mtmd_context_params_default();
	params.use_gpu = use_gpu;
	params.n_threads = threads;
	params.print_timings = false;
	params.warmup = false;
	return mtmd_init_from_file(path, model, params);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// VisionProjector is the image encoder (the GGUF "mmproj" file) paired with
// a generator's model, through llama.cpp's mtmd library. Only describing
// images loads it; `ask` never does, so its ~0.7 GB stays out of the
// answer's memory budget.
type VisionProjector struct {
	ctx *C.struct_mtmd_context
}

// LoadVisionProjector opens path for the model generator runs; the pair must
// come from the same release (Qwen3.5-2B with its own mmproj). Call Close
// before closing the generator.
//
//	projector, err := llamacpp.LoadVisionProjector(generator, "mmproj-Qwen3.5-2B-F16.gguf", true)
func LoadVisionProjector(generator *Generator, path string, useGPU bool) (*VisionProjector, error) {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	threads := C.int(threadCount(0, runtime.NumCPU()))
	ctx := C.cade_load_projector(cPath, generator.loaded.model, C.bool(useGPU), threads)
	if ctx == nil {
		return nil, fmt.Errorf("load vision projector %q: file missing, not an mmproj GGUF, or made for another model", path)
	}
	return &VisionProjector{ctx: ctx}, nil
}

// SupportsImages reports whether the projector encodes images (an mmproj
// may carry only audio).
func (p *VisionProjector) SupportsImages() bool {
	return bool(C.mtmd_support_vision(p.ctx))
}

// Close frees the projector.
func (p *VisionProjector) Close() error {
	C.mtmd_free(p.ctx)
	return nil
}
