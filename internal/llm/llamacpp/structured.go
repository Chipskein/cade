package llamacpp

/*
#include <stdlib.h>
#include "llama.h"
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

const grammarRootRule = "root"

// GenerateStructured replies with text accepted by grammar (GBNF, rule
// "root"), sampling greedily so the output is deterministic.
//
//	json, err := generator.GenerateStructured(ctx, messages, 96, `root ::= "{" ... "}"`)
func (g *Generator) GenerateStructured(ctx context.Context, messages []llm.ChatMessage, maxTokens int, grammar string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	sampler, err := g.newGrammarSampler(grammar)
	if err != nil {
		return "", err
	}
	defer C.llama_sampler_free(sampler)
	tokens, err := g.promptTokens(messages, maxTokens)
	if err != nil {
		return "", err
	}
	if err := g.ingestPrompt(ctx, tokens, llm.GenerationProgress{}); err != nil {
		return "", err
	}
	return g.sampleReply(ctx, sampler, maxTokens, llm.GenerationProgress{})
}

// newGrammarSampler chains the grammar (which masks tokens that would break
// it) with greedy selection.
func (g *Generator) newGrammarSampler(grammar string) (*C.struct_llama_sampler, error) {
	cGrammar, cRoot := C.CString(grammar), C.CString(grammarRootRule)
	defer C.free(unsafe.Pointer(cGrammar))
	defer C.free(unsafe.Pointer(cRoot))
	grammarSampler := C.llama_sampler_init_grammar(g.loaded.vocab, cGrammar, cRoot)
	if grammarSampler == nil {
		return nil, fmt.Errorf("invalid GBNF grammar of %d bytes; expected a %q rule", len(grammar), grammarRootRule)
	}
	chain := C.llama_sampler_chain_init(C.llama_sampler_chain_default_params())
	C.llama_sampler_chain_add(chain, grammarSampler)
	C.llama_sampler_chain_add(chain, C.llama_sampler_init_greedy())
	return chain, nil
}
