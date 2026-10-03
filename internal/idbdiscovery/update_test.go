package idbdiscovery

import (
	"context"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/idbmap"
)

func TestRegenerateTellsTheModelWhatChanged(t *testing.T) {
	current, err := discoverTeams(t, &FakeSchemaGenerator{Replies: teamsReplies(t)})
	if err != nil {
		t.Fatal(err)
	}
	drift := idbmap.Drift{
		Missing: []idbmap.PathShape{{Store: "replychains", Path: "$.messageMap.<id>.content", Kinds: []string{"string"}}},
		Changed: []idbmap.PathShape{{Store: "replychains", Path: "$.messageMap.<id>.originalArrivalTime", Kinds: []string{"string"}}},
	}
	generator := &FakeSchemaGenerator{Replies: teamsReplies(t)}
	found, err := NewDiscoverer(generator, DefaultCatalogLimits).Regenerate(context.Background(), teamsRecords(t), current.Schema, drift)
	if err != nil || found.Schema.Name != "teams-web" || found.Schema.Source != "teams" {
		t.Fatalf("expected the same name and source back, got %+v (err %v)", found.Schema, err)
	}
	for i, prompt := range generator.Prompts {
		question := prompt[len(prompt)-1].Content
		for _, want := range []string{"mudou o formato", "- message_id: $.id", "sumiram: $.messageMap.<id>.content.", "tipo: $.messageMap.<id>.originalArrivalTime (string)"} {
			if !strings.Contains(question, want) {
				t.Errorf("question %d lacks %q:\n%s", i, want, question)
			}
		}
	}
}

func TestDiscoverAddsNoNote(t *testing.T) {
	generator := &FakeSchemaGenerator{Replies: teamsReplies(t)}
	if _, err := discoverTeams(t, generator); err != nil {
		t.Fatal(err)
	}
	if question := generator.Prompts[0][len(generator.Prompts[0])-1].Content; strings.Contains(question, "mudou o formato") {
		t.Fatalf("a first discovery has no previous schema, got:\n%s", question)
	}
}
