package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

// aroundRequest is `cade timeline --around UID [--window D]`.
type aroundRequest struct {
	uid        string
	window     time.Duration
	source     event.Source
	allAuthors bool
	// extra are positional arguments, which --around does not take.
	extra []string
}

func runTimelineAround(ctx context.Context, env commandEnv, request aroundRequest) error {
	if len(request.extra) > 0 {
		return fmt.Errorf(env.language.pick("--around não aceita datas, recebido %q", "--around takes no dates, got %q"), request.extra)
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		near, found, err := timeline.NewLister(store).Around(ctx, request.uid, request.window, request.source)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf(env.language.pick("nenhum evento com UID %q; os UIDs aparecem em `cade ask --json`", "no event with UID %q; UIDs appear in `cade ask --json`"), request.uid)
		}
		if !request.allAuthors {
			near.Events = ownCommitsAround(near.Events, near.Anchor.UID)
		}
		renderAround(env.stdout, env.toolkit.Now().Location(), near, env.language)
		return nil
	})
}

// ownCommitsAround hides colleagues' commits, as the timeline does, but
// never the one asked about.
func ownCommitsAround(events []event.Event, anchor string) []event.Event {
	var kept []event.Event
	for _, ev := range events {
		if ev.UID == anchor || !ev.IsOthersCommit() {
			kept = append(kept, ev)
		}
	}
	return kept
}

func renderAround(out io.Writer, location *time.Location, near timeline.Neighborhood, language Language) {
	fmt.Fprintf(out, language.pick("Em volta de %s (±%s) — %s\n", "Around %s (±%s) — %s\n"), near.Anchor.UID, shortDuration(near.Window), language.count(len(near.Events), eventNoun))
	renderEventLines(out, location, near.Events, near.Anchor.UID)
}

// shortDuration drops the zero units Go prints: 2h, 1h30m, 45m.
func shortDuration(duration time.Duration) string {
	text := duration.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}
