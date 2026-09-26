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
	"math"
	"sync"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

// defaultEmbeddingContextTokens matches the training context of common
// embedding models (nomic-embed-text); the batch must be sized explicitly,
// so "0 = model default" cannot be passed through here.
const defaultEmbeddingContextTokens = 2048

// Embedder produces sentence embeddings with a pooled GGUF embedding model
// (e.g. nomic-embed-text).
type Embedder struct {
	mu         sync.Mutex
	loaded     loadedModel
	dimensions int
	maxTokens  int
}

var _ llm.Embedder = (*Embedder)(nil)

// LoadEmbedder loads an embedding model. Call Close to release it.
//
//	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: "nomic-embed.gguf"})
func LoadEmbedder(opts ModelOptions) (*Embedder, error) {
	if opts.ContextTokens <= 0 {
		opts.ContextTokens = defaultEmbeddingContextTokens
	}
	params := baseContextParams(opts)
	params.embeddings = C.bool(true)
	// Non-causal models must see the whole input in one micro-batch.
	params.n_batch, params.n_ubatch = params.n_ctx, params.n_ctx
	loaded, err := loadModel(opts, params)
	if err != nil {
		return nil, err
	}
	if C.llama_pooling_type(loaded.ctx) == C.LLAMA_POOLING_TYPE_NONE {
		loaded.free()
		return nil, fmt.Errorf("model %q has no pooling; expected a sentence-embedding model (mean/cls/last pooling)", opts.Path)
	}
	return &Embedder{
		loaded:     loaded,
		dimensions: int(C.llama_model_n_embd_out(loaded.model)),
		maxTokens:  int(C.llama_n_ctx(loaded.ctx)),
	}, nil
}

// Embed returns the L2-normalised embedding of text. Input longer than the
// context window is truncated.
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
	tokens = tokens[:min(len(tokens), e.maxTokens)]
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
