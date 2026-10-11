package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/export"
	"github.com/chipskein/cade/internal/storage"
)

var exportedEventNoun = nounForms{"evento exportado", "eventos exportados", "event exported", "events exported"}

// runExport writes what cade keeps as JSON Lines on stdout (#85), with the
// filters of `forget --match`; the count goes to stderr, so stdout is only
// the export.
func runExport(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("export", env.stderr, env.language)
	source := flags.String("source", "", "limita a fonte")
	from := flags.String("from", "", "data inicial (AAAA-MM-DD)")
	to := flags.String("to", "", "data final (AAAA-MM-DD)")
	if _, err := parseCommandFlags(flags, args); err != nil {
		return err
	}
	filter, err := sourceAndDays(*source, *from, *to)
	if err != nil {
		return err
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		return env.exportFrom(ctx, cfg, store, filter)
	})
}

func (env commandEnv) exportFrom(ctx context.Context, cfg config.Config, store storage.EventStore, filter storage.EventFilter) error {
	if err := env.checkExportSource(cfg, filter.Source); err != nil {
		return err
	}
	readable, supported := store.(export.Store)
	if !supported {
		return errors.New(env.language.pick("este banco não pode ser exportado", "this database cannot be exported"))
	}
	count, err := export.Write(ctx, readable, env.stdout, filter)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.stderr, env.language.count(count, exportedEventNoun)+".")
	return nil
}

// checkExportSource rejects a typo in --source, which would export nothing
// without saying why.
func (env commandEnv) checkExportSource(cfg config.Config, source event.Source) error {
	if source == "" {
		return nil
	}
	for _, spec := range env.toolkit.Sources(cfg, nil) {
		if spec.Name == string(source) {
			return nil
		}
	}
	return fmt.Errorf(env.language.pick("fonte %q desconhecida", "unknown source %q"), source)
}
