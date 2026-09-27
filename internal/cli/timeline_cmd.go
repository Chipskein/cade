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
	flags := newFlagSet("timeline", env.stderr, env.language)
	source := flags.String("source", "", env.language.pick("mostra só uma fonte (git, browser, file, teams)", "show one source only (git, browser, file, teams)"))
	allAuthors := flags.Bool("all-authors", false, env.language.pick("inclui commits de outros autores", "include other authors' commits"))
	if err := flags.Parse(args); err != nil {
		return usageError(err)
	}
	days, err := parseTimelineDays(flags.Args(), env.toolkit.Now(), env.language)
	if err != nil {
		return err
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		events, err := timeline.NewLister(store).List(ctx, days, event.Source(*source))
		if err != nil {
			return err
		}
		if !*allAuthors {
			events = ownCommitsOnly(events)
		}
		renderTimeline(env.stdout, days, events, env.language)
		return nil
	})
}

func parseTimelineDays(args []string, now time.Time, language Language) (timeline.DayRange, error) {
	switch len(args) {
	case 1:
		return timeline.ParseDayRange(args[0], "", now)
	case 2:
		return timeline.ParseDayRange(args[0], args[1], now)
	}
	return timeline.DayRange{}, fmt.Errorf(language.pick("esperado DATA ou DATA DATA_FIM, recebido %d argumentos: %q", "expected DATE or DATE END_DATE, got %d arguments: %q"), len(args), args)
}

// withStore loads the config and opens the store for the duration of use.
func (env commandEnv) withStore(ctx context.Context, use func(config.Config, storage.EventStore) error) error {
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	store, err := env.toolkit.OpenStore(ctx, cfg.DatabasePath, env.reportBackup)
	if err != nil {
		return err
	}
	defer store.Close()
	return use(cfg, store)
}

// reportBackup says where a migration saved the copy of the database.
func (env commandEnv) reportBackup(backupPath string) {
	fmt.Fprintf(env.stderr, env.language.pick("Banco atualizado para o novo esquema; cópia da versão anterior em %s\n",
		"Database upgraded to the new schema; copy of the previous version at %s\n"), backupPath)
}

// ownCommitsOnly drops commits known to be someone else's: the timeline is
// the user's activity, and a team repository holds everyone's commits.
func ownCommitsOnly(events []event.Event) []event.Event {
	var kept []event.Event
	for _, ev := range events {
		if !ev.IsOthersCommit() {
			kept = append(kept, ev)
		}
	}
	return kept
}
