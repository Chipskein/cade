package cli

import (
	"context"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

// askModels loads each model on first use: a question the rules read that
// lists events or reports tasks never loads the generator, and a plain
// listing never loads the embedder.
type askModels struct {
	env       commandEnv
	cfg       config.Config
	store     storage.EventStore
	generator ClosableGenerator
	embedder  ClosableEmbedder
	session   *askSession
}

func (env commandEnv) newAskModels(cfg config.Config, store storage.EventStore, session *askSession) *askModels {
	return &askModels{env: env, cfg: cfg, store: store, session: session}
}

// loadedGenerator loads the generator on first call.
func (m *askModels) loadedGenerator() (ClosableGenerator, error) {
	if m.generator != nil {
		return m.generator, nil
	}
	m.session.loadingModels()
	generator, err := m.env.toolkit.LoadGenerator(m.cfg.Generation, m.env.logger)
	if err != nil {
		return nil, err
	}
	m.generator = generator
	return generator, nil
}

// answerer builds the RAG answerer, loading the embedder if needed; the
// stored vectors must come from the configured model. Its generator is
// whatever is loaded: answering needs loadedGenerator first, ranking a
// listing by topic does not.
func (m *askModels) answerer(ctx context.Context) (*rag.Answerer, error) {
	if m.embedder == nil {
		if err := m.env.checkEmbeddingModel(ctx, m.cfg, m.store); err != nil {
			return nil, err
		}
		m.session.loadingModels()
		embedder, err := m.env.toolkit.LoadEmbedder(m.cfg.Embedding, m.env.logger)
		if err != nil {
			return nil, err
		}
		m.embedder = embedder
	}
	deps := rag.Dependencies{Store: m.store, Embedder: m.embedder, Generator: m.generator, Now: m.env.toolkit.Now, Logger: m.env.logger}
	settings, err := ragSettings(m.cfg)
	if err != nil {
		return nil, err
	}
	return rag.NewAnswerer(deps, settings), nil
}

func (m *askModels) close() {
	if m.embedder != nil {
		m.embedder.Close()
	}
	if m.generator != nil {
		m.generator.Close()
	}
}
