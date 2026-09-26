package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

// listForPlan answers "as mensagens da Ana ontem" from the database: every
// event matching the filters, not the top-k most similar. A topic ("sobre
// redis") narrows the list by similarity.
func (env commandEnv) listForPlan(ctx context.Context, store storage.EventStore, models *askModels, plan askPlan, session *askSession) error {
	days := *plan.question.Days
	events, err := timeline.NewLister(store).List(ctx, days, plan.question.Source)
	if err != nil {
		return err
	}
	events, matched, unknown := plan.question.Criteria.Apply(events)
	reportPeople(env.stderr, matched, unknown)
	if events, err = env.narrowByTopic(ctx, models, plan.topic, events); err != nil {
		return err
	}
	session.status.clear()
	renderTimeline(env.stdout, days, events)
	return nil
}

func (env commandEnv) narrowByTopic(ctx context.Context, models *askModels, topic string, events []event.Event) ([]event.Event, error) {
	if topic == "" || len(events) == 0 {
		return events, nil
	}
	answerer, err := models.answerer()
	if err != nil {
		return nil, err
	}
	return answerer.FilterByTopic(ctx, topic, events)
}

// reportPeople tells which names filtered and which matched nobody (a
// misread name, a company) and were ignored.
func reportPeople(out io.Writer, matched, unknown []string) {
	if len(matched) > 0 {
		fmt.Fprintf(out, "Filtrando por pessoa: %s\n", strings.Join(matched, ", "))
	}
	if len(unknown) > 0 {
		fmt.Fprintf(out, "Sem correspondência, ignorado: %s\n", strings.Join(unknown, ", "))
	}
}
