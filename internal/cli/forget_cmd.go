package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

var removedEventNoun = nounForms{"evento removido", "eventos removidos", "event removed", "events removed"}

func runForget(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("forget", env.stderr, env.language)
	uid := flags.String("uid", "", "UID do evento")
	match := flags.String("match", "", "texto a localizar nos eventos")
	source := flags.String("source", "", "limita a fonte")
	from := flags.String("from", "", "data inicial (AAAA-MM-DD)")
	to := flags.String("to", "", "data final (AAAA-MM-DD)")
	yes := flags.Bool("yes", false, "confirma sem perguntar")
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if *uid != "" {
		return env.forgetUID(ctx, *uid)
	}
	if *match != "" {
		return env.forgetMatch(ctx, *match, *source, *from, *to, *yes)
	}
	if len(positional) != 1 {
		return errors.New(env.language.pick("use cade forget <fonte> ou --uid UID ou --match TEXTO", "use cade forget <source>, --uid UID or --match TEXT"))
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		if _, err := ingest.FindSource(env.toolkit.Sources(cfg, nil), positional[0]); err != nil {
			return err
		}
		removed, err := store.DeleteSource(ctx, event.Source(positional[0]))
		if err != nil {
			return err
		}
		fmt.Fprint(env.stdout, forgottenLine(positional[0], removed, env.language))
		return nil
	})
}

func (env commandEnv) forgetUID(ctx context.Context, uid string) error {
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		deleted, err := store.DeleteEvent(ctx, uid)
		if err != nil {
			return err
		}
		if deleted {
			fmt.Fprintln(env.stdout, env.language.pick("evento removido.", "event removed."))
		} else {
			fmt.Fprintln(env.stdout, env.language.pick("UID não encontrado.", "UID not found."))
		}
		return nil
	})
}

func (env commandEnv) forgetMatch(ctx context.Context, text, source, from, to string, yes bool) error {
	filter := storage.EventFilter{Source: event.Source(source)}
	var err error
	if filter.From, err = parseForgetDate(from); err != nil {
		return err
	}
	if filter.To, err = parseForgetDate(to); err != nil {
		return err
	}
	if !filter.To.IsZero() {
		filter.To = filter.To.Add(24 * time.Hour)
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		matches, err := store.EventsContaining(ctx, text, filter)
		if err != nil {
			return err
		}
		for _, ev := range matches {
			fmt.Fprintf(env.stdout, "%s\t%s\t%s\n", ev.UID, ev.Timestamp.Format("2006-01-02"), ev.Headline())
		}
		if len(matches) == 0 {
			return nil
		}
		if !yes {
			if !env.toolkit.StderrIsTerminal {
				return fmt.Errorf("%s", env.language.pick("use --yes fora de um terminal", "use --yes outside a terminal"))
			}
			fmt.Fprint(env.stderr, env.language.pick("Apagar estes eventos? [s/N] ", "Delete these events? [y/N] "))
			answer, readErr := bufio.NewReader(env.toolkit.Stdin).ReadString('\n')
			if readErr != nil {
				return readErr
			}
			if !strings.EqualFold(strings.TrimSpace(answer), env.language.pick("s", "y")) {
				return nil
			}
		}
		return env.deleteMatches(ctx, store, matches)
	})
}

// deleteMatches removes the matched events, one store call each — the part
// of `forget --match` that can take a while on a large match set — showing
// a throttled "done/total (%) · ETA" line while it runs.
func (env commandEnv) deleteMatches(ctx context.Context, store storage.EventStore, matches []event.Event) error {
	status := statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal}
	defer status.clear()
	tracker := newETATracker(env.toolkit.Now, etaWindow)
	lastShown := env.toolkit.Now()
	for i, ev := range matches {
		if _, err := store.DeleteEvent(ctx, ev.UID); err != nil {
			return err
		}
		tracker.advance()
		done := i + 1
		if now := env.toolkit.Now(); done == len(matches) || now.Sub(lastShown) >= progressInterval(status.interactive) {
			lastShown = now
			status.show(deleteMatchesLine(done, len(matches), tracker, env.language))
		}
	}
	status.clear()
	fmt.Fprintln(env.stdout, env.language.count(len(matches), removedEventNoun)+".")
	return nil
}

// deleteMatchesLine is "Apagando eventos: 12/50 (24%) · ETA ~5s".
func deleteMatchesLine(done, total int, tracker *etaTracker, language Language) string {
	prefix := language.pick("Apagando eventos: ", "Deleting events: ")
	return prefix + progressBar(done, total) + etaSuffix(tracker, total-done)
}

func parseForgetDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q must use YYYY-MM-DD", value)
	}
	return date, nil
}

func forgottenLine(source string, removed int, language Language) string {
	return fmt.Sprintf(language.pick("%s: %s. Rode `cade ingest %s` para ingerir de novo.\n", "%s: %s. Run `cade ingest %s` to ingest again.\n"), source, language.count(removed, removedEventNoun), source)
}
