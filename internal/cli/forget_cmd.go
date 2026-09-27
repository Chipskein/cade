package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

var removedEventNoun = nounForms{"evento removido", "eventos removidos", "event removed", "events removed"}

// runForget deletes one source's events so it can be re-ingested with an
// improved collector; deduplication would otherwise keep the old events.
func runForget(ctx context.Context, env commandEnv, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf(env.language.pick("informe uma fonte: cade forget <git|browser|file|teams>, recebido %q", "name one source: cade forget <git|browser|file|teams>, got %q"), args)
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		if _, err := ingest.FindSource(env.toolkit.Sources(cfg), args[0]); err != nil {
			return err
		}
		removed, err := store.DeleteSource(ctx, event.Source(args[0]))
		if err != nil {
			return err
		}
		fmt.Fprint(env.stdout, forgottenLine(args[0], removed, env.language))
		return nil
	})
}

// forgottenLine is "git: 1 evento removido. Rode `cade ingest git` …".
func forgottenLine(source string, removed int, language Language) string {
	return fmt.Sprintf(language.pick("%s: %s. Rode `cade ingest %s` para ingerir de novo.\n", "%s: %s. Run `cade ingest %s` to ingest again.\n"),
		source, language.count(removed, removedEventNoun), source)
}
