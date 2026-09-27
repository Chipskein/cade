package cli

import (
	"flag"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func TestParseCommandFlagsAcceptsFlagsAnywhere(t *testing.T) {
	cases := []struct {
		args, positional []string
		source           string
		all              bool
	}{
		{[]string{"--source", "git", "ontem"}, []string{"ontem"}, "git", false},
		{[]string{"ontem", "--source", "git"}, []string{"ontem"}, "git", false},
		{[]string{"ontem", "--all", "hoje", "--source=file"}, []string{"ontem", "hoje"}, "file", true},
		{[]string{"ontem", "--", "--all"}, []string{"ontem", "--all"}, "", false},
		{[]string{"--", "-x", "y"}, []string{"-x", "y"}, "", false},
		{nil, nil, "", false},
	}
	for _, c := range cases {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		source, all := flags.String("source", "", ""), flags.Bool("all", false, "")
		positional, err := parseCommandFlags(flags, c.args)
		if err != nil || !slices.Equal(positional, c.positional) || *source != c.source || *all != c.all {
			t.Errorf("%q: got %q source=%q all=%v err=%v, expected %q source=%q all=%v", c.args, positional, *source, *all, err, c.positional, c.source, c.all)
		}
	}
}

func TestParseCommandFlagsRejectsUnknownFlagAfterArguments(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if _, err := parseCommandFlags(flags, []string{"ontem", "--nope"}); err != errUsage {
		t.Fatalf("expected a usage error, got %v", err)
	}
}

// Acceptance (phase 16): `cade timeline ontem --source git` works.
func TestTimelineAcceptsFlagAfterDate(t *testing.T) {
	world := newFakeWorld()
	visit := event.Event{UID: "v", Source: event.SourceBrowser, Timestamp: time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC), Content: "página visitada"}
	world.store.Events = []event.Event{commitBy("a", "Eu", event.AuthorshipMine), visit}
	code, stdout, stderr := world.run("timeline", "ontem", "--source", "git")
	if code != 0 || !strings.Contains(stdout, "commit de Eu") || strings.Contains(stdout, "página visitada") {
		t.Fatalf("expected only the git event, got %d %q %q", code, stdout, stderr)
	}
}

func TestAskAcceptsFlagsAfterQuestion(t *testing.T) {
	world := newFakeWorld()
	world.run("ask", "o que fiz?", "--source", "browser")
	if world.store.LastQuery.Source != event.SourceBrowser {
		t.Fatalf("expected --source after the question to filter, got %+v", world.store.LastQuery)
	}
}
