package llamacpp

/*
#include "llama.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Reranker scores how well a document answers a query with a cross-encoder
// GGUF (rank pooling, e.g. bge-reranker-v2-m3). Phase 17 measures it on the
// retrieval suite before any command uses it.
type Reranker struct {
	mu        sync.Mutex
	loaded    loadedModel
	maxTokens int
}

// LoadReranker loads a reranking model. Call Close to release it.
//
//	reranker, err := llamacpp.LoadReranker(llamacpp.ModelOptions{Path: "bge-reranker-v2-m3.gguf", ContextTokens: 1024})
func LoadReranker(opts ModelOptions) (*Reranker, error) {
	loaded, err := loadModel(opts, func(model *C.struct_llama_model) C.struct_llama_context_params {
		params := embeddingContextParams(opts, int(C.llama_model_n_ctx_train(model)))
		params.pooling_type = C.LLAMA_POOLING_TYPE_RANK
		return params
	})
	if err != nil {
		return nil, err
	}
	if C.llama_model_n_cls_out(loaded.model) < 1 {
		loaded.free()
		return nil, fmt.Errorf("model %q has no classification head; expected a reranker (rank pooling)", opts.Path)
	}
	return &Reranker{loaded: loaded, maxTokens: int(C.llama_n_ctx(loaded.ctx))}, nil
}

// Score returns the relevance of document to query; higher is closer. The
// document is cut to fit the context with the query.
func (r *Reranker) Score(query, document string) (float32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens, err := r.pairTokens(query, document)
	if err != nil {
		return 0, err
	}
	if err := r.loaded.decodeSequence(tokens); err != nil {
		return 0, err
	}
	score := C.llama_get_embeddings_seq(r.loaded.ctx, 0)
	if score == nil {
		return 0, fmt.Errorf("read rank score: llama.cpp returned no score for sequence 0")
	}
	return *(*float32)(unsafe.Pointer(score)), nil
}

// pairTokens builds <bos> query <eos> <sep> document <eos>, the pair layout
// llama.cpp's server uses when the model ships no rerank template.
func (r *Reranker) pairTokens(query, document string) ([]C.llama_token, error) {
	plain := tokenizeOptions{}
	queryTokens, err := r.loaded.tokenizeText(query, plain)
	if err != nil {
		return nil, err
	}
	documentTokens, err := r.loaded.tokenizeText(document, plain)
	if err != nil {
		return nil, err
	}
	head := append(r.optional(C.llama_vocab_get_add_bos(r.loaded.vocab), C.llama_vocab_bos(r.loaded.vocab)), queryTokens...)
	head = append(head, r.optional(C.llama_vocab_get_add_eos(r.loaded.vocab), r.endToken())...)
	head = append(head, r.optional(C.llama_vocab_get_add_sep(r.loaded.vocab), C.llama_vocab_sep(r.loaded.vocab))...)
	tail := r.optional(C.llama_vocab_get_add_eos(r.loaded.vocab), r.endToken())
	room := r.maxTokens - len(head) - len(tail)
	if room <= 0 {
		return nil, fmt.Errorf("rerank query of %d tokens leaves no room in a %d-token context", len(queryTokens), r.maxTokens)
	}
	return append(append(head, documentTokens[:min(len(documentTokens), room)]...), tail...), nil
}

func (r *Reranker) optional(wanted C.bool, token C.llama_token) []C.llama_token {
	if !bool(wanted) {
		return nil
	}
	return []C.llama_token{token}
}

// endToken is EOS, or SEP for vocabularies without one.
func (r *Reranker) endToken() C.llama_token {
	if eos := C.llama_vocab_eos(r.loaded.vocab); eos != C.LLAMA_TOKEN_NULL {
		return eos
	}
	return C.llama_vocab_sep(r.loaded.vocab)
}

// Close frees the model and its context.
func (r *Reranker) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loaded.free()
	return nil
}
