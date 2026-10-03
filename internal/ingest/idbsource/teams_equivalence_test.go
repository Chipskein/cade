package idbsource

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/teamssource"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// The reviewed Teams schema must produce the same core fields as the
// Teams collector written in code: the acceptance criterion of #19.
const (
	teamsSchemaPath = "../../../testdata/idb-schemas/teams-web.json"
	teamsSampleDir  = "../../../testdata/teams-formats/2026-09.leveldb"
)

// coreFields are what equivalence means here: the conversation title and
// kind come from another store in the Teams collector and may differ.
type coreFields struct {
	UID       string
	Timestamp time.Time
	Sender    string
	Text      string
}

func loadSchema(t *testing.T, path string) idbmap.Schema {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := idbmap.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return schema
}

func collectCore(t *testing.T, collector ingest.EventCollector) []coreFields {
	t.Helper()
	var core []coreFields
	err := collector.CollectEvents(context.Background(), func(ev event.Event) error {
		core = append(core, coreFields{UID: ev.UID, Timestamp: ev.Timestamp, Sender: ev.Message().Sender, Text: ev.Message().Text})
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	slices.SortFunc(core, func(a, b coreFields) int { return strings.Compare(a.UID, b.UID) })
	return core
}

func assertEquivalent(t *testing.T, reader webstore.Reader, dir string) {
	t.Helper()
	generic, err := NewCollector(webstore.Readers{reader}, dir, loadSchema(t, teamsSchemaPath))
	if err != nil {
		t.Fatal(err)
	}
	want := collectCore(t, teamssource.NewCollector(reader.Read, dir))
	got := collectCore(t, generic)
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("schema events differ from the Teams collector's:\nschema: %+v\nteams:  %+v", got, want)
	}
	t.Logf("%d events with the same UID, time, sender and text", len(got))
}

func TestTeamsSchemaMatchesTheTeamsCollectorOnTheRealSample(t *testing.T) {
	assertEquivalent(t, indexeddb.ChromiumReader{}, teamsSampleDir)
}

func TestTeamsSchemaMatchesTheTeamsCollectorOnEdgeCases(t *testing.T) {
	assertEquivalent(t, &testfakes.FakeStoreReader{StoreKind: webstore.KindIndexedDB, Records: teamsEdgeCases()}, "/p/https_teams.microsoft.com_0.indexeddb.leveldb")
}

// teamsEdgeCases holds one message per rule the Teams collector applies.
func teamsEdgeCases() []indexeddb.Record {
	messages := obj(
		"1", teamsMessage("1", "RichText/Html", "<p>Olá <b>time</b></p>", "imDisplayName", str("Ana Souza"), "version", str("1727280000001")),
		"2", teamsMessage("2", "Text", "só token", "fromDisplayNameInToken", str("  Bruno  ")),
		"3", teamsMessage("3", "Text", "pelo perfil", "creator", str("8:orgid:carla")),
		"4", teamsMessage("4", "Text", "sem nome", "creator", str("8:orgid:ninguem")),
		"5", teamsMessage("5", "Text", "pendente", "originalArrivalTime", num(0), "clientArrivalTime", num(1727283600000)),
		"6", teamsMessage("6", "Text", "notificação", "threadType", str("streamofnotifications")),
		"7", teamsMessage("7", "Text", "apagada", "deletionInfo", obj("deleteTime", num(1))),
		"8", teamsMessage("8", "RichText/Html", "<p> </p>"),
		"9", teamsMessage("9", "ThreadActivity/AddMember", "<addmember/>"),
		"10", obj("id", str("10"), "conversationId", str("19:c@thread.v2"), "messageType", str("Text"), "content", str("sem hora")),
	)
	return []indexeddb.Record{
		{Namespace: "Teams:replychain-manager:x", Container: "replychains", Value: obj("id", str("19:c@thread.v2"), "messageMap", messages)},
		{Namespace: "Teams:profiles:x", Container: "profiles", Value: obj("mri", str("8:orgid:carla"), "displayName", str(" Carla Dias "))},
	}
}

func teamsMessage(id, messageType, content string, pairs ...any) *v8value.Value {
	base := []any{"id", str(id), "conversationId", str("19:c@thread.v2"), "messageType", str(messageType),
		"content", str(content), "originalArrivalTime", num(1727280000000)}
	return obj(append(base, pairs...)...)
}
