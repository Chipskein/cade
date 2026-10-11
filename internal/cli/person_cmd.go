package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// personNamed is the one full name key a typed name matches; none or
// several stop `--person` before it touches anything, since erasing or
// exporting must name exactly one person (#86).
func (env commandEnv) personNamed(ctx context.Context, store storage.EventStore, name string) (string, error) {
	keys, err := store.PeopleNamed(ctx, name)
	switch {
	case err != nil:
		return "", err
	case len(keys) == 0:
		return "", errors.New(env.language.pick(fmt.Sprintf("ninguém chamado %q", name), fmt.Sprintf("no one named %q", name)))
	case len(keys) > 1:
		return "", errors.New(env.language.pick(
			fmt.Sprintf("%q pode ser %s; use o nome completo", name, strings.Join(keys, ", ")),
			fmt.Sprintf("%q may be %s; use the full name", name, strings.Join(keys, ", "))))
	}
	return keys[0], nil
}

// eventsOfPersonNamed are the events about the one person name means.
func (env commandEnv) eventsOfPersonNamed(ctx context.Context, store storage.EventStore, name string) ([]event.Event, error) {
	key, err := env.personNamed(ctx, store, name)
	if err != nil {
		return nil, err
	}
	return store.EventsOfPerson(ctx, key)
}

// forgetPerson lists everything about the person and deletes it once
// confirmed; the entities left without events go with them (PRIVACY,
// rule 4).
func (env commandEnv) forgetPerson(ctx context.Context, name string, yes bool) error {
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		events, err := env.eventsOfPersonNamed(ctx, store, name)
		if err != nil {
			return err
		}
		return env.reviewAndDelete(ctx, store, events, yes)
	})
}
