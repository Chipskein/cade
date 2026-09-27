package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

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
		fmt.Fprintf(env.stdout, env.language.pick("%d eventos de %s removidos. Rode `cade ingest %s` para ingerir de novo.\n",
			"%d %s events removed. Run `cade ingest %s` to ingest again.\n"), removed, args[0], args[0])
		return nil
	})
}
