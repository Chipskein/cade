// Package llm declares the local-model capabilities the rest of the program
// needs. Implementations must run fully on this machine (RNF1).
package llm

import "context"

// Embedder turns text into a vector for similarity search.
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// ChatRole is the speaker of a chat message.
type ChatRole string

const (
	RoleSystem    ChatRole = "system"
	RoleUser      ChatRole = "user"
	RoleAssistant ChatRole = "assistant"
)

// ChatMessage is one turn of a conversation passed to a Generator.
type ChatMessage struct {
	Role    ChatRole
	Content string
}

// GenerationProgress observes a generation. On CPU, reading a long prompt
// takes most of the time before the first token, so both phases are
// reported. Nil fields are ignored.
type GenerationProgress struct {
	// PromptProcessed reports how many prompt tokens have been read.
	PromptProcessed func(done, total int)
	// TokenGenerated receives each piece of the reply as it is produced,
	// always as complete UTF-8.
	TokenGenerated func(piece string)
}

// NotifyPrompt calls PromptProcessed when set.
func (p GenerationProgress) NotifyPrompt(done, total int) {
	if p.PromptProcessed != nil {
		p.PromptProcessed(done, total)
	}
}

// NotifyToken calls TokenGenerated when set.
func (p GenerationProgress) NotifyToken(piece string) {
	if p.TokenGenerated != nil {
		p.TokenGenerated(piece)
	}
}

// Generator produces a reply to a conversation. Cancelling ctx stops the
// generation between tokens.
type Generator interface {
	Generate(ctx context.Context, messages []ChatMessage, maxTokens int, progress GenerationProgress) (string, error)
}

// StructuredGenerator produces output constrained by a GBNF grammar, so a
// small model can be trusted to emit parseable JSON. Sampling is greedy:
// the same question always yields the same structure.
type StructuredGenerator interface {
	GenerateStructured(ctx context.Context, messages []ChatMessage, maxTokens int, grammar string) (string, error)
}
