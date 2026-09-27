package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

// askFlags holds the raw filter flags of `cade ask`. Flags always win over
// what the model reads from the question.
type askFlags struct {
	source    *string
	from      *string
	to        *string
	noFilters *bool
	json      *bool
}

func registerAskFlags(flags *flag.FlagSet, language Language) askFlags {
	return askFlags{
		source: flags.String("source", "", language.pick("busca só em uma fonte (git, browser, file, teams)", "search one source only (git, browser, file, teams)")),
		from:   flags.String("from", "", language.pick("primeiro dia considerado (AAAA-MM-DD, hoje, ontem)", "first day considered (YYYY-MM-DD, hoje, ontem)")),
		to:     flags.String("to", "", language.pick("último dia considerado (padrão: hoje quando --from é dado)", "last day considered (default: today when --from is given)")),
		noFilters: flags.Bool("no-filters", false, language.pick("não interpretar filtros na pergunta (período, pessoas, fonte)",
			"do not read filters from the question (period, people, source)")),
		json: flags.Bool("json", false, language.pick("saída em JSON: plano, resultado e a referência de cada evento usado",
			"JSON output: plan, result and the reference of every event used")),
	}
}

func runAsk(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("ask", env.stderr, env.toolkit.Language)
	filters := registerAskFlags(flags, env.toolkit.Language)
	if err := flags.Parse(args); err != nil {
		return usageError(err)
	}
	text := strings.Join(flags.Args(), " ")
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("informe a pergunta: cade ask \"o que fiz ontem?\"")
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		return env.askWithStore(ctx, cfg, store, text, filters)
	})
}

// askWithStore interprets the question, then lists or answers.
func (env commandEnv) askWithStore(ctx context.Context, cfg config.Config, store storage.EventStore, text string, filters askFlags) error {
	session := newAskSession(env, *filters.json)
	models := env.newAskModels(cfg, store, session)
	defer models.close()
	query, err := env.resolveAskQuery(ctx, models, text, filters, session)
	if err != nil {
		return err
	}
	switch query.Mode {
	case queryplan.ModeList:
		return env.listForQuery(ctx, store, models, query, session)
	case queryplan.ModeTasks:
		return env.tasksForQuery(ctx, cfg, store, query, session)
	}
	return env.answerForQuery(ctx, models, query, session)
}

func (env commandEnv) answerForQuery(ctx context.Context, models *askModels, query queryplan.Query, session *askSession) error {
	if _, err := models.loadedGenerator(); err != nil {
		return err
	}
	answerer, err := models.answerer(ctx)
	if err != nil {
		return err
	}
	answer, err := answerer.Answer(ctx, query, session.observer())
	if err != nil {
		session.status.clear()
		return err
	}
	if session.jsonOutput {
		return session.writeReport(query, func(report *askReport) { report.Answer = answerReportOf(answer) })
	}
	session.render(answer, env.toolkit.Now().Location())
	return nil
}

// parseOptionalDays returns nil when neither bound is given. A lone --to
// means "everything up to that day"; a lone --from runs until today.
func parseOptionalDays(from, to string, now time.Time) (*timeline.DayRange, error) {
	if from == "" && to == "" {
		return nil, nil
	}
	if from == "" {
		from = "1970-01-01"
	}
	if to == "" {
		to = "hoje"
	}
	days, err := timeline.ParseDayRange(from, to, now)
	if err != nil {
		return nil, err
	}
	return &days, nil
}

func ragSettings(cfg config.Config) (rag.Settings, error) {
	mode, err := rag.ParseMode(cfg.Retrieval.Mode)
	return rag.Settings{
		TopK:              cfg.Retrieval.TopK,
		MaxDistance:       cfg.Retrieval.MaxDistance,
		MaxBestDistance:   cfg.Retrieval.MaxBestDistance,
		QueryPrefix:       cfg.Embedding.QueryPrefix,
		MaxAnswerTokens:   cfg.Retrieval.MaxAnswerTokens,
		Mode:              mode,
		MaxFilteredEvents: cfg.Retrieval.MaxFilteredEvents,
	}, err
}
