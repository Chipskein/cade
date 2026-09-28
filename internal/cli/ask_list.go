package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

// listForQuery answers "as mensagens da Ana ontem" from the database: every
// event matching the filters, not the top-k most similar. A topic ("sobre
// redis") narrows the list by similarity.
func (env commandEnv) listForQuery(ctx context.Context, store storage.EventStore, models *askModels, query queryplan.Query, session *askSession) error {
	days := *query.Days
	events, err := timeline.NewLister(store).List(ctx, days, query.Source)
	if err != nil {
		return err
	}
	if query.OwnCommitsOnly {
		events = ownCommitsOnly(events)
	}
	events = listing.InFolder(events, query.Folder)
	events, matched, unknown := query.Criteria.Apply(events)
	reportPeople(env.stderr, matched, nil, env.language)
	reportNamesAsText(env.stderr, unknown, env.language)
	events = eventsMentioningAll(events, unknown)
	if events, err = env.narrowByTopic(ctx, models, query.Topic, events); err != nil {
		return err
	}
	if session.jsonOutput {
		return session.writeReport(query, func(report *askReport) { report.Events = eventReferences(events) })
	}
	session.status.clear()
	renderTimeline(env.stdout, days, events, env.language)
	return nil
}

func (env commandEnv) narrowByTopic(ctx context.Context, models *askModels, topic string, events []event.Event) ([]event.Event, error) {
	if topic == "" || len(events) == 0 {
		return events, nil
	}
	answerer, err := models.answerer(ctx)
	if err != nil {
		return nil, err
	}
	return answerer.FilterByTopic(ctx, topic, events)
}

// reportPeople tells which names filtered and which matched nobody (a
// misread name, a company) and were ignored.
func reportPeople(out io.Writer, matched, unknown []string, language Language) {
	if len(matched) > 0 {
		fmt.Fprintf(out, language.pick("Filtrando por pessoa: %s\n", "Filtering by person: %s\n"), strings.Join(matched, ", "))
	}
	if len(unknown) > 0 {
		fmt.Fprintf(out, language.pick("Sem correspondência, ignorado: %s\n", "No match, ignored: %s\n"), strings.Join(unknown, ", "))
	}
}
