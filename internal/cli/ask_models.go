package cli

import (
	"context"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

// askModels holds the generator (always needed: it interprets the
// question) and loads the embedder only on first use, so a plain listing
// never pays for it.
type askModels struct {
	env       commandEnv
	cfg       config.Config
	store     storage.EventStore
	generator ClosableGenerator
	embedder  ClosableEmbedder
	session   *askSession
}

func (env commandEnv) loadAskModels(cfg config.Config, store storage.EventStore, session *askSession) (*askModels, error) {
	session.loadingModels()
	generator, err := env.toolkit.LoadGenerator(cfg.Generation)
	if err != nil {
		return nil, err
	}
	return &askModels{env: env, cfg: cfg, store: store, generator: generator, session: session}, nil
}

// answerer builds the RAG answerer, loading the embedder if needed; the
// stored vectors must come from the configured model.
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
	return rag.NewAnswerer(deps, ragSettings(m.cfg)), nil
}

func (m *askModels) close() {
	if m.embedder != nil {
		m.embedder.Close()
	}
	m.generator.Close()
}
