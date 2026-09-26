package llamacpp

/*
#include <stdlib.h>
#include "llama.h"

// Builds a single-sequence batch with every position marked as output,
// which pooled embedding models require.
static struct llama_batch cade_sequence_batch(const llama_token * tokens, int32_t count) {
	struct llama_batch batch = llama_batch_init(count, 0, 1);
	for (int32_t i = 0; i < count; i++) {
		batch.token[i] = tokens[i];
		batch.pos[i] = i;
		batch.n_seq_id[i] = 1;
		batch.seq_id[i][0] = 0;
		batch.logits[i] = 1;
	}
	batch.n_tokens = count;
	return batch;
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"math"
	"sync"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

// fallbackEmbeddingContextTokens is used only when neither the options nor
// the model state a context length.
const fallbackEmbeddingContextTokens = 512

// embeddingContext is the context the embedder runs with: the configured
// one (0 = unset), capped at the length the model was trained on. Beyond
// it, positions are ones the model never saw: nomic-embed-text-v2-moe is
// trained on 512 tokens and cade used to configure 2048, degrading every
// long note and message.
func embeddingContext(configured, trained int) int {
	switch {
	case trained <= 0 && configured <= 0:
		return fallbackEmbeddingContextTokens
	case trained <= 0:
		return configured
	case configured <= 0:
		return trained
	}
	return min(configured, trained)
}

// Embedder produces sentence embeddings with a pooled GGUF embedding model
// (e.g. nomic-embed-text).
type Embedder struct {
	mu         sync.Mutex
	loaded     loadedModel
	dimensions int
	maxTokens  int
	logger     *slog.Logger
}

var _ llm.Embedder = (*Embedder)(nil)

// LoadEmbedder loads an embedding model. Call Close to release it.
//
//	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: "nomic-embed.gguf"})
func LoadEmbedder(opts ModelOptions) (*Embedder, error) {
	loaded, err := loadModel(opts, func(model *C.struct_llama_model) C.struct_llama_context_params {
		return embeddingContextParams(opts, int(C.llama_model_n_ctx_train(model)))
	})
	if err != nil {
		return nil, err
	}
	if C.llama_pooling_type(loaded.ctx) == C.LLAMA_POOLING_TYPE_NONE {
		loaded.free()
		return nil, fmt.Errorf("model %q has no pooling; expected a sentence-embedding model (mean/cls/last pooling)", opts.Path)
	}
	embedder := &Embedder{loaded: loaded, dimensions: int(C.llama_model_n_embd_out(loaded.model)),
		maxTokens: int(C.llama_n_ctx(loaded.ctx)), logger: opts.logger()}
	embedder.logger.Debug("embedding context", "configured", opts.ContextTokens,
		"trained", int(C.llama_model_n_ctx_train(loaded.model)), "effective", embedder.maxTokens)
	return embedder, nil
}

// ContextTokens is the effective context: inputs beyond it are cut, so
// vectors computed with different contexts are not interchangeable.
func (e *Embedder) ContextTokens() int {
	return e.maxTokens
}

func embeddingContextParams(opts ModelOptions, trained int) C.struct_llama_context_params {
	opts.ContextTokens = embeddingContext(opts.ContextTokens, trained)
	params := baseContextParams(opts)
	params.embeddings = C.bool(true)
	// Non-causal models must see the whole input in one micro-batch.
	params.n_batch, params.n_ubatch = params.n_ctx, params.n_ctx
	return params
}

// Embed returns the L2-normalised embedding of text. Input longer than the
// context window is truncated, and the cut is logged at debug level.
func (e *Embedder) Embed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	tokens, err := e.loaded.tokenize(text, false)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("embed text %q: produced no tokens", text)
	}
	if len(tokens) > e.maxTokens {
		e.logger.Debug("embedding input truncated", "tokens", len(tokens), "limit", e.maxTokens)
		tokens = tokens[:e.maxTokens]
	}
	if err := e.decode(tokens); err != nil {
		return nil, err
	}
	return e.readPooledEmbedding()
}

func (e *Embedder) decode(tokens []C.llama_token) error {
	e.loaded.clearMemory()
	batch := C.cade_sequence_batch(&tokens[0], C.int32_t(len(tokens)))
	defer C.llama_batch_free(batch)
	if status := C.llama_decode(e.loaded.ctx, batch); status != 0 {
		return fmt.Errorf("decode %d tokens for embedding: llama.cpp status %d", len(tokens), status)
	}
	return nil
}

func (e *Embedder) readPooledEmbedding() ([]float32, error) {
	pooled := C.llama_get_embeddings_seq(e.loaded.ctx, 0)
	if pooled == nil {
		return nil, fmt.Errorf("read pooled embedding: llama.cpp returned no vector for sequence 0")
	}
	raw := unsafe.Slice((*float32)(unsafe.Pointer(pooled)), e.dimensions)
	return normalize(raw), nil
}

// Close frees the model and its context.
func (e *Embedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.loaded.free()
	return nil
}

// normalize copies vector scaled to unit length, so cosine distance and dot
// product agree regardless of the model's output scale.
func normalize(vector []float32) []float32 {
	var sumSquares float64
	for _, value := range vector {
		sumSquares += float64(value) * float64(value)
	}
	scale := float32(1)
	if sumSquares > 0 {
		scale = float32(1 / math.Sqrt(sumSquares))
	}
	normalized := make([]float32, len(vector))
	for i, value := range vector {
		normalized[i] = value * scale
	}
	return normalized
}
