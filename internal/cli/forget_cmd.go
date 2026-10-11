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

// forgetOptions are the flags of `cade forget`.
type forgetOptions struct {
	uid, match, source, from, to, person *string
	yes, backups                         *bool
}

func runForget(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("forget", env.stderr, env.language)
	options := forgetOptions{uid: flags.String("uid", "", "UID do evento"), match: flags.String("match", "", "texto a localizar nos eventos"),
		source: flags.String("source", "", "limita a fonte"), from: flags.String("from", "", "data inicial (AAAA-MM-DD)"),
		to: flags.String("to", "", "data final (AAAA-MM-DD)"), person: flags.String("person", "", "nome da pessoa: apaga tudo sobre ela"),
		yes: flags.Bool("yes", false, "confirma sem perguntar"), backups: flags.Bool("backups", false, "apaga as cópias do banco")}
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if *options.backups {
		return env.forgetCopies(*options.yes)
	}
	if err := env.forgetChosen(ctx, options, positional); err != nil {
		return err
	}
	return env.warnAboutCopies()
}

// forgetChosen runs the kind of forget the flags chose.
func (env commandEnv) forgetChosen(ctx context.Context, options forgetOptions, positional []string) error {
	switch {
	case *options.uid != "":
		return env.forgetUID(ctx, *options.uid)
	case *options.match != "":
		return env.forgetMatch(ctx, *options.match, *options.source, *options.from, *options.to, *options.yes)
	case *options.person != "":
		return env.forgetPerson(ctx, *options.person, *options.yes)
	case len(positional) != 1:
		return errors.New(env.language.pick("use cade forget <fonte> ou --uid UID ou --match TEXTO", "use cade forget <source>, --uid UID or --match TEXT"))
	}
	return env.forgetSource(ctx, positional[0])
}

func (env commandEnv) forgetSource(ctx context.Context, source string) error {
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		if _, err := ingest.FindSource(env.toolkit.Sources(cfg, nil), source); err != nil {
			return err
		}
		removed, err := store.DeleteSource(ctx, event.Source(source))
		if err != nil {
			return err
		}
		fmt.Fprint(env.stdout, forgottenLine(source, removed, env.language))
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
	filter, err := sourceAndDays(source, from, to)
	if err != nil {
		return err
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		matches, err := store.EventsContaining(ctx, text, filter)
		if err != nil {
			return err
		}
		return env.reviewAndDelete(ctx, store, matches, yes)
	})
}

// reviewAndDelete lists the events, asks before deleting unless yes, and
// deletes them; nothing listed, nothing asked.
func (env commandEnv) reviewAndDelete(ctx context.Context, store storage.EventStore, events []event.Event, yes bool) error {
	for _, ev := range events {
		fmt.Fprintf(env.stdout, "%s\t%s\t%s\n", ev.UID, ev.Timestamp.Format("2006-01-02"), ev.Headline())
	}
	if len(events) == 0 {
		return nil
	}
	confirmed, err := env.confirm(yes, env.language.pick("Apagar estes eventos? [s/N] ", "Delete these events? [y/N] "))
	if err != nil || !confirmed {
		return err
	}
	return env.deleteMatches(ctx, store, events)
}

// confirm asks question on a terminal; yes answers it in scripts.
func (env commandEnv) confirm(yes bool, question string) (bool, error) {
	if yes {
		return true, nil
	}
	if !env.toolkit.StderrIsTerminal {
		return false, fmt.Errorf("%s", env.language.pick("use --yes fora de um terminal", "use --yes outside a terminal"))
	}
	fmt.Fprint(env.stderr, question)
	answer, err := bufio.NewReader(env.toolkit.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(answer), env.language.pick("s", "y")), nil
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

// sourceAndDays is the filter of `forget --match` and `export`: a source
// and the days from and to (YYYY-MM-DD, both included); empty is
// unbounded.
func sourceAndDays(source, from, to string) (storage.EventFilter, error) {
	filter := storage.EventFilter{Source: event.Source(source)}
	var err error
	if filter.From, err = parseForgetDate(from); err != nil {
		return storage.EventFilter{}, err
	}
	if filter.To, err = parseForgetDate(to); err != nil {
		return storage.EventFilter{}, err
	}
	if !filter.To.IsZero() {
		filter.To = filter.To.Add(24 * time.Hour)
	}
	return filter, nil
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
