package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// checkWorld has a whatsapp schema discovered on chat records, with its
// directory configured.
func checkWorld(t *testing.T) *fakeWorld {
	t.Helper()
	world := discoverWorld(t, discoverFieldsReply)
	if code, _, stderr := world.run("idb-discover", "--name", "whatsapp", whatsappDir); code != 0 {
		t.Fatalf("discover: %d %s", code, stderr)
	}
	world.cfg.Sources.IndexedDBDirs = map[string][]string{"whatsapp": {whatsappDir}}
	return world
}

// renameField renames a field in every record, as an app update would.
func renameField(records []indexeddb.Record, from, to string) []indexeddb.Record {
	for _, record := range records {
		for i := range record.Value.Properties {
			if record.Value.Properties[i].Key == from {
				record.Value.Properties[i].Key = to
			}
		}
	}
	return records
}

func TestIDBCheckReportsAMatchingSchema(t *testing.T) {
	code, stdout, stderr := checkWorld(t).run("idb-check")
	if code != 0 || !strings.Contains(stdout, "whatsapp (https+++web.whatsapp.com): ok, 2 mensagens") {
		t.Fatalf("expected ok, got %d:\n%s%s", code, stdout, stderr)
	}
}

func TestIDBCheckFailsOnDriftWithoutUpdate(t *testing.T) {
	world := checkWorld(t)
	world.indexedDBRecords = renameField([]indexeddb.Record{chatRecord("A"), chatRecord("B")}, "t", "ts")
	loadsBefore := world.generatorLoads
	code, stdout, stderr := world.run("idb-check", "whatsapp")
	if code != 1 || !strings.Contains(stdout, "mudou: 1 caminho sumiu") || !strings.Contains(stdout, "message $.t") || !strings.Contains(stderr, "não resolvida") {
		t.Fatalf("expected the drift reported and exit 1, got %d:\n%s%s", code, stdout, stderr)
	}
	if world.generatorLoads != loadsBefore {
		t.Fatal("checking without --update must not load the model")
	}
}

func TestIDBCheckUpdateReplacesAnAcceptedSchema(t *testing.T) {
	world := checkWorld(t)
	world.indexedDBRecords = renameField([]indexeddb.Record{chatRecord("A"), chatRecord("B")}, "t", "ts")
	world.generator.StructuredReplies = []string{discoverStoreReply, strings.Replace(discoverFieldsReply, `["$.t"]`, `["$.ts"]`, 1)}
	code, stdout, stderr := world.run("idb-check", "--update")
	if code != 0 || !strings.Contains(stdout, "substituído pela revisão 2: 2 mensagens, 100% dos UIDs mantidos") {
		t.Fatalf("expected the schema replaced, got %d:\n%s%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(world.cfg.Sources.IndexedDBSchemaDir, "history", "whatsapp.1.json")); err != nil {
		t.Fatalf("expected revision 1 kept in history: %v", err)
	}
	if code, stdout, _ := world.run("idb-discover", "--rollback", "--name", "whatsapp"); code != 0 || !strings.Contains(stdout, "voltou à revisão 1") {
		t.Fatalf("expected the rollback to revision 1, got %d %s", code, stdout)
	}
}

// A regenerated schema that reads the conversation elsewhere would give
// indexed messages new UIDs: it is kept as a candidate, not swapped in.
func TestIDBCheckUpdateKeepsARefusedSchemaAsCandidate(t *testing.T) {
	world := checkWorld(t)
	world.indexedDBRecords = renameField([]indexeddb.Record{chatRecord("A"), chatRecord("B")}, "body", "texto")
	for _, record := range world.indexedDBRecords {
		record.Value.Properties = append(record.Value.Properties, v8value.Property{Key: "chat", Value: &v8value.Value{Kind: v8value.KindString, Text: "outro@c.us"}})
	}
	world.generator.StructuredReplies = []string{discoverStoreReply, strings.Replace(discoverFieldsReply, `["$.from"]`, `["$.chat"]`, 1)}
	code, stdout, _ := world.run("idb-check", "--update", "whatsapp")
	if code != 1 || !strings.Contains(stdout, "não substituído (2 mensagens, 0% dos UIDs mantidos)") {
		t.Fatalf("expected the replacement refused, got %d:\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(world.cfg.Sources.IndexedDBSchemaDir, "whatsapp.candidate.json")); err != nil {
		t.Fatalf("expected the candidate saved: %v", err)
	}
}

func TestIDBCheckUpdateRekeysWhenAsked(t *testing.T) {
	world := checkWorld(t)
	world.indexedDBRecords = renameField([]indexeddb.Record{chatRecord("A"), chatRecord("B")}, "body", "texto")
	for _, record := range world.indexedDBRecords {
		record.Value.Properties = append(record.Value.Properties, v8value.Property{Key: "chat", Value: &v8value.Value{Kind: v8value.KindString, Text: "outro@c.us"}})
	}
	world.generator.StructuredReplies = []string{discoverStoreReply, strings.Replace(discoverFieldsReply, `["$.from"]`, `["$.chat"]`, 1)}
	code, stdout, stderr := world.run("idb-check", "--update", "--rekey", "whatsapp")
	if code != 0 || !strings.Contains(stdout, "substituído pela revisão 2 com UIDs novos") || !strings.Contains(stdout, "before-rekey") {
		t.Fatalf("expected the rekey and the replacement, got %d:\n%s%s", code, stdout, stderr)
	}
	if len(world.store.Rekeyed) != 2 {
		t.Fatalf("expected both messages rekeyed, got %v", world.store.Rekeyed)
	}
}

func TestIDBCheckWithoutDirectories(t *testing.T) {
	world := checkWorld(t)
	world.cfg.Sources.IndexedDBDirs = nil
	if code, stdout, _ := world.run("idb-check"); code != 0 || !strings.Contains(stdout, "nenhum diretório") {
		t.Fatalf("expected a note about the missing directory, got %d %s", code, stdout)
	}
}

func TestIDBDiscoverRollbackWithoutHistory(t *testing.T) {
	if code, _, stderr := checkWorld(t).run("idb-discover", "--rollback", "--name", "whatsapp"); code != 1 || !strings.Contains(stderr, "no earlier revision") {
		t.Fatalf("expected no earlier revision, got %d %q", code, stderr)
	}
}
