package pacing

import (
	"context"

	"github.com/chipskein/cade/internal/llm"
)

// ClosableEmbedder is an embedder holding a model that must be released.
type ClosableEmbedder interface {
	llm.Embedder
	Close() error
}

// ClosableDescriber is a vision model that must be released.
type ClosableDescriber interface {
	llm.ImageDescriber
	Close() error
}

// PacedEmbedder rests after every embedding. Embed takes no context, so
// the one the run was started with ends its rests.
type PacedEmbedder struct {
	ctx   context.Context
	inner ClosableEmbedder
	cycle *DutyCycle
}

// NewPacedEmbedder paces inner with cycle for the run of ctx.
//
//	embedder = pacing.NewPacedEmbedder(ctx, embedder, cycle)
func NewPacedEmbedder(ctx context.Context, inner ClosableEmbedder, cycle *DutyCycle) *PacedEmbedder {
	return &PacedEmbedder{ctx: ctx, inner: inner, cycle: cycle}
}

func (p *PacedEmbedder) Embed(text string) ([]float32, error) {
	var vector []float32
	err := p.cycle.Run(p.ctx, func() error {
		var embedErr error
		vector, embedErr = p.inner.Embed(text)
		return embedErr
	})
	return vector, err
}

func (p *PacedEmbedder) Close() error { return p.inner.Close() }

// PacedDescriber rests after every image description.
type PacedDescriber struct {
	inner ClosableDescriber
	cycle *DutyCycle
}

// NewPacedDescriber paces inner with cycle.
//
//	describer = pacing.NewPacedDescriber(describer, cycle)
func NewPacedDescriber(inner ClosableDescriber, cycle *DutyCycle) *PacedDescriber {
	return &PacedDescriber{inner: inner, cycle: cycle}
}

func (p *PacedDescriber) DescribeImage(ctx context.Context, image llm.RGBImage, instructions string, maxTokens int) (string, error) {
	var reply string
	err := p.cycle.Run(ctx, func() error {
		var describeErr error
		reply, describeErr = p.inner.DescribeImage(ctx, image, instructions, maxTokens)
		return describeErr
	})
	return reply, err
}

func (p *PacedDescriber) Close() error { return p.inner.Close() }
