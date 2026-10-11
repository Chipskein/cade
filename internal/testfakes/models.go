package testfakes

import (
	"context"
	"errors"
	"strings"

	"github.com/chipskein/cade/internal/llm"
)

// FakeEmbedder returns a fixed-size vector derived from the text length and
// records every input it saw.
type FakeEmbedder struct {
	Inputs   []string
	FailWith error
	Closed   bool
	// Vector, when set, is returned for every input (to control distances).
	Vector []float32
}

func (f *FakeEmbedder) Embed(text string) ([]float32, error) {
	f.Inputs = append(f.Inputs, text)
	if f.FailWith != nil {
		return nil, f.FailWith
	}
	if f.Vector != nil {
		return f.Vector, nil
	}
	return []float32{float32(len(text)), 1}, nil
}

func (f *FakeEmbedder) Close() error {
	f.Closed = true
	return nil
}

// FakeGenerator returns Reply, streaming it word by word through the
// progress callbacks, and records the conversation it was given.
type FakeGenerator struct {
	Reply string
	// StructuredReply is returned by GenerateStructured (the question plan).
	StructuredReply string
	// StructuredReplies, when set, answer GenerateStructured in order
	// instead (schema discovery asks twice); Grammars records each grammar.
	StructuredReplies []string
	Grammars          []string
	LastMessages      []llm.ChatMessage
	Calls             int
	FailWith          error
	Closed            bool
}

func (f *FakeGenerator) Generate(ctx context.Context, messages []llm.ChatMessage, _ int, progress llm.GenerationProgress) (string, error) {
	f.Calls++
	f.LastMessages = messages
	if f.FailWith != nil || ctx.Err() != nil {
		return "", errors.Join(f.FailWith, ctx.Err())
	}
	progress.NotifyPrompt(1, 1)
	for _, piece := range strings.SplitAfter(f.Reply, " ") {
		progress.NotifyToken(piece)
	}
	return f.Reply, nil
}

// noFilterPlan is what a real planner returns for a plain question.
const noFilterPlan = `{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null}`

// GenerateStructured returns the next of StructuredReplies, else
// StructuredReply, or a no-filter plan.
func (f *FakeGenerator) GenerateStructured(ctx context.Context, _ []llm.ChatMessage, _ int, grammar string) (string, error) {
	f.Grammars = append(f.Grammars, grammar)
	if len(f.StructuredReplies) > 0 {
		reply := f.StructuredReplies[0]
		f.StructuredReplies = f.StructuredReplies[1:]
		return reply, ctx.Err()
	}
	if f.StructuredReply == "" {
		return noFilterPlan, ctx.Err()
	}
	return f.StructuredReply, ctx.Err()
}

func (f *FakeGenerator) Close() error {
	f.Closed = true
	return nil
}

// FakeImageDescriber returns Reply for every image and records the sizes
// it was given.
type FakeImageDescriber struct {
	Reply    string
	Images   []llm.RGBImage
	FailWith error
	Closed   bool
}

func (f *FakeImageDescriber) DescribeImage(ctx context.Context, image llm.RGBImage, _ string, _ int) (string, error) {
	f.Images = append(f.Images, llm.RGBImage{Width: image.Width, Height: image.Height})
	if f.FailWith != nil || ctx.Err() != nil {
		return "", errors.Join(f.FailWith, ctx.Err())
	}
	return f.Reply, nil
}

func (f *FakeImageDescriber) Close() error {
	f.Closed = true
	return nil
}
