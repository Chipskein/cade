package llamacpp

/*
#include <stdlib.h>
#include "llama.h"
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/chipskein/cade/internal/llm"
)

// Low temperature keeps answers close to the retrieved evidence (RF4.3);
// min-p trims the tail that produces invented details.
const (
	samplingTemperature = 0.2
	samplingMinP        = 0.05
	samplingSeed        = 42
)

// Generator runs a chat (instruction-tuned) GGUF model.
type Generator struct {
	mu        sync.Mutex
	loaded    loadedModel
	modelPath string
	sampler   *C.struct_llama_sampler
	batchSize int
	cache     promptCache
	states    promptStateStore
	logger    *slog.Logger
}

var (
	_ llm.Generator           = (*Generator)(nil)
	_ llm.StructuredGenerator = (*Generator)(nil)
)

// LoadGenerator loads a chat model. Call Close to release it.
//
//	generator, err := llamacpp.LoadGenerator(llamacpp.ModelOptions{Path: "Qwen3.5-2B-Q4_K_M.gguf", ContextTokens: 8192})
func LoadGenerator(opts ModelOptions) (*Generator, error) {
	params := baseContextParams(opts)
	loaded, err := loadModel(opts, func(*C.struct_llama_model) C.struct_llama_context_params { return params })
	if err != nil {
		return nil, err
	}
	if C.llama_model_chat_template(loaded.model, nil) == nil {
		loaded.free()
		return nil, fmt.Errorf("model %q has no chat template; expected an instruction-tuned GGUF", opts.Path)
	}
	states, err := newPromptStateStore(opts.PromptStateDir, opts, int(C.llama_n_ctx(loaded.ctx)))
	if err != nil {
		loaded.free()
		return nil, err
	}
	return &Generator{
		loaded:    loaded,
		modelPath: opts.Path,
		sampler:   newSampler(),
		batchSize: int(params.n_batch),
		states:    states,
		logger:    opts.logger(),
	}, nil
}

func newSampler() *C.struct_llama_sampler {
	chain := C.llama_sampler_chain_init(C.llama_sampler_chain_default_params())
	C.llama_sampler_chain_add(chain, C.llama_sampler_init_min_p(samplingMinP, 1))
	C.llama_sampler_chain_add(chain, C.llama_sampler_init_temp(samplingTemperature))
	C.llama_sampler_chain_add(chain, C.llama_sampler_init_dist(samplingSeed))
	return chain
}

// promptChunkTokens is how many prompt tokens are decoded per call. Smaller
// than the batch size only so progress can be reported while a long prompt
// is read; llama.cpp tracks positions across calls.
const promptChunkTokens = 256

// Generate replies to messages with at most maxTokens new tokens.
func (g *Generator) Generate(ctx context.Context, messages []llm.ChatMessage, maxTokens int, progress llm.GenerationProgress) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	tokens, err := g.promptTokens(messages, maxTokens)
	if err != nil {
		return "", err
	}
	if err := g.ingestPrompt(ctx, tokens, progress); err != nil {
		return "", err
	}
	return g.sampleReply(ctx, g.sampler, maxTokens, progress)
}

func (g *Generator) promptTokens(messages []llm.ChatMessage, maxTokens int) ([]C.llama_token, error) {
	prompt, err := applyChatTemplate(g.loaded.model, messages, true)
	if err != nil {
		return nil, err
	}
	tokens, err := g.loaded.tokenize(prompt, true)
	if err != nil {
		return nil, err
	}
	return tokens, g.checkFits(len(tokens), maxTokens)
}

func (g *Generator) checkFits(promptTokens, maxTokens int) error {
	contextTokens := int(C.llama_n_ctx(g.loaded.ctx))
	if promptTokens+maxTokens > contextTokens {
		return fmt.Errorf("prompt of %d tokens plus %d reply tokens exceeds the %d-token context; raise generation context_tokens or lower retrieval top_k",
			promptTokens, maxTokens, contextTokens)
	}
	return nil
}

func (g *Generator) ingestPrompt(ctx context.Context, tokens []C.llama_token, progress llm.GenerationProgress) error {
	C.llama_sampler_reset(g.sampler)
	cached := g.cache.reuse(g.loaded, tokens)
	chunk := min(promptChunkTokens, g.batchSize)
	for start := cached; start < len(tokens); start += chunk {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(start+chunk, len(tokens))
		if err := g.decodeTokens(tokens[start:end]); err != nil {
			return err
		}
		progress.NotifyPrompt(end, len(tokens))
	}
	return nil
}

func (g *Generator) decodeTokens(tokens []C.llama_token) error {
	batch := C.llama_batch_get_one(&tokens[0], C.int32_t(len(tokens)))
	if status := C.llama_decode(g.loaded.ctx, batch); status != 0 {
		g.cache.reset()
		return fmt.Errorf("decode %d tokens: llama.cpp status %d", len(tokens), status)
	}
	g.cache.add(tokens)
	return nil
}

func (g *Generator) sampleReply(ctx context.Context, sampler *C.struct_llama_sampler, maxTokens int, progress llm.GenerationProgress) (string, error) {
	stream := utf8Streamer{emit: progress.NotifyToken}
	for range maxTokens {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		token := C.llama_sampler_sample(sampler, g.loaded.ctx, -1)
		if C.llama_vocab_is_eog(g.loaded.vocab, token) {
			break
		}
		stream.write(g.loaded.tokenPiece(token))
		if err := g.decodeTokens([]C.llama_token{token}); err != nil {
			return "", err
		}
	}
	return stream.finish(), nil
}

// Close frees the sampler, model and context.
func (g *Generator) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	C.llama_sampler_free(g.sampler)
	g.loaded.free()
	return nil
}
