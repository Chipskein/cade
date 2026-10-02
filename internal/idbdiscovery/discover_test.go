package idbdiscovery

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/llm"
)

// FakeSchemaGenerator answers the discovery's calls in order and records
// what the model would have read.
type FakeSchemaGenerator struct {
	Replies  []string
	FailWith error
	Prompts  [][]llm.ChatMessage
	Grammars []string
}

func (f *FakeSchemaGenerator) GenerateStructured(_ context.Context, messages []llm.ChatMessage, _ int, grammar string) (string, error) {
	f.Prompts, f.Grammars = append(f.Prompts, messages), append(f.Grammars, grammar)
	if f.FailWith != nil {
		return "", f.FailWith
	}
	if len(f.Prompts) > len(f.Replies) {
		return "", errors.New("FakeSchemaGenerator: no reply left")
	}
	return f.Replies[len(f.Prompts)-1], nil
}

const teamsFieldsReply = `{"message_id": ["$.id"], "conversation_id": ["$.conversationId"], "sent_at": ["$.originalArrivalTime", "$.clientArrivalTime"], "time_format": "unix_ms", "sender": ["$.imDisplayName"], "sender_id": ["$.creator"], "conversation": [], "text": ["$.content"], "text_format": "html_text", "sent_by_me": ["$.isSentByCurrentUser"], "keep": {"path": "$.messageType", "in": ["Text", "RichText/Html"]}, "sender_lookup": {"store": "PROFILES", "key": "$.creator", "match": "$.mri", "value": "$.displayName"}}`

func teamsRecords(t *testing.T) []indexeddb.Record {
	t.Helper()
	records, err := indexeddb.ReadDirectory(teamsSampleDir)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// teamsReplies answers with the labels the Teams sample's catalog gives.
func teamsReplies(t *testing.T) []string {
	t.Helper()
	catalog := teamsCatalog(t)
	chains, profiles := storeNamed(t, catalog, "replychains"), storeNamed(t, catalog, "profiles")
	return []string{
		`{"store": "` + chains.Label + `", "each": "$.messageMap.<id>"}`,
		strings.Replace(teamsFieldsReply, "PROFILES", profiles.Label, 1),
	}
}

func discoverTeams(t *testing.T, generator *FakeSchemaGenerator) (Discovery, error) {
	t.Helper()
	return NewDiscoverer(generator, DefaultCatalogLimits).Discover(context.Background(), teamsRecords(t), "teams-web", "teams")
}

func TestDiscoverBuildsASchemaThatMapsTheRecords(t *testing.T) {
	found, err := discoverTeams(t, &FakeSchemaGenerator{Replies: teamsReplies(t)})
	if err != nil || found.Events == 0 || found.Tally.Mapped != found.Events {
		t.Fatalf("expected mapped events, got %+v (err %v)", found, err)
	}
	schema := found.Schema
	if schema.Name != "teams-web" || schema.Source != "teams" || schema.Records.Store != "replychains" || schema.Records.Each != "$.messageMap.<id>" {
		t.Fatalf("unexpected header %+v", schema)
	}
	if !strings.HasPrefix(schema.Records.DatabasePrefix, "Teams:replychain-manager:") || schema.Fields[idbmap.FieldSentAt].Transform != idbmap.TransformUnixMS {
		t.Fatalf("unexpected records %+v / sent_at %+v", schema.Records, schema.Fields[idbmap.FieldSentAt])
	}
	sender := schema.Fields[idbmap.FieldSender]
	if sender.Lookup == nil || sender.Lookup.Store != "profiles" || sender.Default != unknownSender || schema.Fields[idbmap.FieldText].Transform != idbmap.TransformHTMLText {
		t.Fatalf("unexpected sender %+v / text %+v", sender, schema.Fields[idbmap.FieldText])
	}
}

func TestDiscoverAsksForTheStoreThenTheFields(t *testing.T) {
	generator := &FakeSchemaGenerator{Replies: teamsReplies(t)}
	if _, err := discoverTeams(t, generator); err != nil {
		t.Fatal(err)
	}
	if len(generator.Grammars) != 2 || !strings.Contains(generator.Grammars[0], "each-") || !strings.Contains(generator.Grammars[1], "time-path") {
		t.Fatalf("expected the store grammar then the fields grammar, got %d calls", len(generator.Grammars))
	}
	fields := generator.Prompts[1][len(generator.Prompts[1])-1].Content
	if !strings.Contains(fields, "$.originalArrivalTime") || strings.Contains(fields, "$.messageMap.<id>.originalArrivalTime") {
		t.Fatalf("expected item paths in the fields question, got:\n%s", fields)
	}
}

// What the model reads stays masked: no id, timestamp or long number.
func TestDiscoverPromptsCarryNoLongNumbers(t *testing.T) {
	generator := &FakeSchemaGenerator{Replies: teamsReplies(t)}
	if _, err := discoverTeams(t, generator); err != nil {
		t.Fatal(err)
	}
	longNumber := regexp.MustCompile(`\d{6,}`)
	for _, prompt := range generator.Prompts {
		if question := prompt[len(prompt)-1].Content; longNumber.MatchString(question) {
			t.Fatalf("a long number reached the prompt: %q", longNumber.FindString(question))
		}
	}
}

func TestDiscoverReturnsSchemasThatMapNothingForReview(t *testing.T) {
	replies := teamsReplies(t)
	replies[1] = strings.Replace(replies[1], `"in": ["Text", "RichText/Html"]`, `"in": ["Nada"]`, 1)
	found, err := discoverTeams(t, &FakeSchemaGenerator{Replies: replies})
	if !errors.Is(err, ErrNoMessages) || found.Schema.Name != "teams-web" || found.Tally.Kept != 0 {
		t.Fatalf("expected ErrNoMessages with the schema kept, got %+v (err %v)", found, err)
	}
}

func TestDiscoverReportsModelFailures(t *testing.T) {
	failure := errors.New("model not loaded")
	if _, err := discoverTeams(t, &FakeSchemaGenerator{FailWith: failure}); !errors.Is(err, failure) {
		t.Errorf("expected the model error, got %v", err)
	}
	if _, err := discoverTeams(t, &FakeSchemaGenerator{Replies: []string{"{store"}}); err == nil || !strings.Contains(err.Error(), "{store") {
		t.Errorf("expected the error to quote the reply, got %v", err)
	}
	if _, err := discoverTeams(t, &FakeSchemaGenerator{Replies: []string{`{"store": "S9", "each": null}`}}); err == nil || !strings.Contains(err.Error(), "S9") {
		t.Errorf("expected an unknown label error, got %v", err)
	}
}

func TestDiscoverWithoutStores(t *testing.T) {
	generator := &FakeSchemaGenerator{}
	if _, err := NewDiscoverer(generator, DefaultCatalogLimits).Discover(context.Background(), nil, "x", "x"); err == nil || len(generator.Prompts) != 0 {
		t.Fatalf("expected an error before asking the model, got %v", err)
	}
}

// A record per message and no body: the shape of an app that encrypts the
// text, like WhatsApp Web.
func TestDiscoverMetadataOnlyStore(t *testing.T) {
	catalog := BuildCatalog(firefoxRecords(t), DefaultCatalogLimits)
	message := storeNamed(t, catalog, "message")
	generator := &FakeSchemaGenerator{Replies: []string{
		`{"store": "` + message.Label + `", "each": null}`,
		`{"message_id": ["$.id"], "conversation_id": ["$.from"], "sent_at": ["$.t"], "time_format": "unix_s", "sender": [], "sender_id": ["$.author._serialized"], "conversation": [], "text": [], "text_format": "plain", "sent_by_me": [], "keep": null, "sender_lookup": null}`,
	}}
	found, err := NewDiscoverer(generator, DefaultCatalogLimits).Discover(context.Background(), firefoxRecords(t), "whatsapp", "whatsapp")
	if err != nil || found.Events != 1 {
		t.Fatalf("expected the decoded message mapped, got %+v (err %v)", found, err)
	}
	if _, hasText := found.Schema.Fields[idbmap.FieldText]; hasText || found.Schema.Records.Each != "" {
		t.Fatalf("expected no text field and no each, got %+v", found.Schema)
	}
}
