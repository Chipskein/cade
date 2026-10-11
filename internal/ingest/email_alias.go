package ingest

import (
	"context"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// A commit by "F. Rocha <fernanda.rocha@example.com>" is Fernanda Rocha's,
// but only the e-mail says so. With aliases on, each commit carries the
// name read from its author's e-mail, which the people filter and the
// entities treat as the author. It is a new kind of personal data, so it
// is off until the user turns it on, and turning it off deletes it
// (PRIVACY, rule 1; #24).

// WithEmailAliases turns the author aliases on or off.
func (p *Pipeline) WithEmailAliases(enabled bool) *Pipeline {
	p.emailAliases = enabled
	return p
}

// aliased is ev with the alias the option asks for: the name read from
// the author's e-mail when on and it differs from the author's, none
// otherwise. Other events pass unchanged.
func (p *Pipeline) aliased(ev event.Event) event.Event {
	if ev.Source != event.SourceGit {
		return ev
	}
	commit, alias := ev.Commit(), ""
	if p.emailAliases {
		alias = listing.NameFromEmail(commit.Email)
	}
	if listing.NameKey(alias) == listing.NameKey(commit.Author) {
		alias = ""
	}
	return ev.WithAuthorAlias(alias)
}

// syncEmailAliases rewrites the stored commits whose alias the option
// changes: on, it fills the commits stored before; off, it deletes every
// alias. Text and vectors are reused, nothing is embedded again.
func (p *Pipeline) syncEmailAliases(ctx context.Context) error {
	everything := storage.EventFilter{From: time.Unix(0, 0), To: p.now().AddDate(1, 0, 0), Source: event.SourceGit}
	commits, err := p.store.EventsMatching(ctx, everything)
	if err != nil {
		return err
	}
	report := Report{}
	for _, stored := range commits {
		if err := p.realias(ctx, stored, &report); err != nil {
			return err
		}
	}
	p.logger.Debug("commit aliases", "enabled", p.emailAliases, "changed", report.Updated)
	return nil
}

func (p *Pipeline) realias(ctx context.Context, stored event.Event, report *Report) error {
	wanted := p.aliased(stored)
	if wanted.Commit().AuthorAlias == stored.Commit().AuthorAlias {
		return nil
	}
	chunks, err := p.chunksFor(ctx, p.store, wanted)
	if err != nil {
		return err
	}
	return p.update(ctx, p.store, wanted, chunks, report)
}
