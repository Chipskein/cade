package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

func runTimeline(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("timeline", env.stderr)
	source := flags.String("source", "", "mostra só uma fonte (git, browser, file)")
	if err := flags.Parse(args); err != nil {
		return errUsage
	}
	days, err := parseTimelineDays(flags.Args(), env.toolkit.Now())
	if err != nil {
		return err
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		events, err := timeline.NewLister(store).List(ctx, days, event.Source(*source))
		if err != nil {
			return err
		}
		renderTimeline(env.stdout, days, events)
		return nil
	})
}

func parseTimelineDays(args []string, now time.Time) (timeline.DayRange, error) {
	switch len(args) {
	case 1:
		return timeline.ParseDayRange(args[0], "", now)
	case 2:
		return timeline.ParseDayRange(args[0], args[1], now)
	}
	return timeline.DayRange{}, fmt.Errorf("esperado DATA ou DATA DATA_FIM, recebido %d argumentos: %q", len(args), args)
}

// withStore loads the config and opens the store for the duration of use.
func (env commandEnv) withStore(ctx context.Context, use func(config.Config, storage.EventStore) error) error {
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	store, err := env.toolkit.OpenStore(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	return use(cfg, store)
}
