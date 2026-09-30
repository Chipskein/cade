// Package llamacpp runs GGUF models in-process through a thin cgo binding to
// llama.cpp. No server or network access is involved (RNF1). Each loaded
// model stays in memory for the life of the value, so repeated calls in one
// run never reload it (RNF5.2).
package llamacpp

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/llama.cpp/include -I${SRCDIR}/../../../third_party/llama.cpp/ggml/include
#include <stdlib.h>
#include "llama.h"

static void cade_discard_log(enum ggml_log_level level, const char * text, void * user_data) {
	(void)level; (void)text; (void)user_data;
}

static void cade_init_backend(void) {
	llama_log_set(cade_discard_log, NULL);
	llama_backend_init();
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"
)

// ModelOptions configures how a GGUF file is loaded and run.
type ModelOptions struct {
	Path string
	// ContextTokens is the context window; 0 uses the model's training size.
	ContextTokens int
	// Threads is the CPU thread count; 0 lets llama.cpp decide.
	Threads int
	// GPULayers is how many layers to offload; ignored by CPU-only builds.
	GPULayers int
	// Logger receives debug records (effective context, truncated inputs);
	// nil discards them.
	Logger *slog.Logger
	// PromptStateDir keeps the generator's saved prompt prefixes (see
	// prompt_state.go); empty disables them. Embedders ignore it.
	PromptStateDir string
}

func (o ModelOptions) logger() *slog.Logger {
	if o.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return o.Logger
}

var initBackendOnce sync.Once

// loadedModel pairs a model with the context that runs it.
type loadedModel struct {
	model *C.struct_llama_model
	ctx   *C.struct_llama_context
	vocab *C.struct_llama_vocab
}

// loadModel loads the file, then builds the context from it: some context
// settings depend on the model (its training context length).
func loadModel(opts ModelOptions, contextParams func(*C.struct_llama_model) C.struct_llama_context_params) (loadedModel, error) {
	initBackendOnce.Do(func() { C.cade_init_backend() })
	model, err := loadModelFile(opts)
	if err != nil {
		return loadedModel{}, err
	}
	params := contextParams(model)
	ctx := C.llama_init_from_model(model, params)
	if ctx == nil {
		C.llama_model_free(model)
		return loadedModel{}, fmt.Errorf("create llama.cpp context for %q with %d context tokens", opts.Path, params.n_ctx)
	}
	return loadedModel{model: model, ctx: ctx, vocab: C.llama_model_get_vocab(model)}, nil
}

func loadModelFile(opts ModelOptions) (*C.struct_llama_model, error) {
	params := C.llama_model_default_params()
	params.n_gpu_layers = C.int32_t(opts.GPULayers)
	cPath := C.CString(opts.Path)
	defer C.free(unsafe.Pointer(cPath))
	model := C.llama_model_load_from_file(cPath, params)
	if model == nil {
		return nil, fmt.Errorf("load GGUF model %q: file missing or not a valid GGUF model", opts.Path)
	}
	return model, nil
}

func baseContextParams(opts ModelOptions) C.struct_llama_context_params {
	params := C.llama_context_default_params()
	params.n_ctx = C.uint32_t(opts.ContextTokens)
	threads := C.int32_t(threadCount(opts.Threads, runtime.NumCPU()))
	params.n_threads, params.n_threads_batch = threads, threads
	return params
}

// threadCount defaults to one thread per physical core. llama.cpp's own
// default is 4; SMT siblings add little to matrix math (on a 6-core/12-thread
// Ryzen 5 5500, 6 threads beat both 4 and 12).
func threadCount(configured, logicalCPUs int) int {
	if configured > 0 {
		return configured
	}
	return max(1, logicalCPUs/2)
}

func (m loadedModel) free() {
	C.llama_free(m.ctx)
	C.llama_model_free(m.model)
}

func (m loadedModel) clearMemory() {
	C.llama_memory_clear(C.llama_get_memory(m.ctx), C.bool(true))
}

// forgetFrom drops the memory from position onward, keeping the prefix;
// false when the model cannot remove part of its memory.
func (m loadedModel) forgetFrom(position int) bool {
	return bool(C.llama_memory_seq_rm(C.llama_get_memory(m.ctx), 0, C.llama_pos(position), -1))
}

// tokenize converts text to tokens, growing the buffer when llama.cpp reports
// (as a negative count) that it needs more room.
func (m loadedModel) tokenize(text string, parseSpecial bool) ([]C.llama_token, error) {
	return m.tokenizeText(text, tokenizeOptions{addSpecial: true, parseSpecial: parseSpecial})
}

// tokenizeOptions: addSpecial adds the model's BOS/EOS around the text;
// parseSpecial reads special-token markup in it.
type tokenizeOptions struct {
	addSpecial   bool
	parseSpecial bool
}

func (m loadedModel) tokenizeText(text string, opts tokenizeOptions) ([]C.llama_token, error) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	tokens := make([]C.llama_token, len(text)+8)
	count := m.tokenizeInto(cText, len(text), tokens, opts)
	if count < 0 {
		tokens = make([]C.llama_token, -count)
		count = m.tokenizeInto(cText, len(text), tokens, opts)
	}
	if count < 0 {
		return nil, fmt.Errorf("tokenize %d bytes of text: llama.cpp returned %d", len(text), count)
	}
	return tokens[:count], nil
}

func (m loadedModel) tokenizeInto(cText *C.char, length int, tokens []C.llama_token, opts tokenizeOptions) int {
	return int(C.llama_tokenize(m.vocab, cText, C.int32_t(length),
		&tokens[0], C.int32_t(len(tokens)), C.bool(opts.addSpecial), C.bool(opts.parseSpecial)))
}

func (m loadedModel) tokenPiece(token C.llama_token) []byte {
	buffer := make([]byte, 64)
	length := m.tokenPieceInto(token, buffer)
	if length < 0 {
		buffer = make([]byte, -length)
		length = m.tokenPieceInto(token, buffer)
	}
	return buffer[:max(length, 0)]
}

func (m loadedModel) tokenPieceInto(token C.llama_token, buffer []byte) int {
	return int(C.llama_token_to_piece(m.vocab, token,
		(*C.char)(unsafe.Pointer(&buffer[0])), C.int32_t(len(buffer)), 0, C.bool(false)))
}
