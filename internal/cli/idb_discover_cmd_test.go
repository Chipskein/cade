package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

const (
	discoverStoreReply  = `{"store": "S1", "each": null}`
	discoverFieldsReply = `{"message_id": ["$.id"], "conversation_id": ["$.from"], "sent_at": ["$.t"], "time_format": "unix_s", "sender": [], "sender_id": [], "conversation": [], "text": ["$.body"], "text_format": "plain", "sent_by_me": [], "keep": null, "sender_lookup": null}`
	whatsappDir         = "/home/ana/.floorp/p/storage/default/https+++web.whatsapp.com/idb"
)

func chatRecord(id string) indexeddb.Record {
	text := func(value string) *v8value.Value { return &v8value.Value{Kind: v8value.KindString, Text: value} }
	value := &v8value.Value{Kind: v8value.KindObject, Properties: []v8value.Property{
		{Key: "id", Value: text(id)}, {Key: "from", Value: text("55@c.us")}, {Key: "body", Value: text("oi")},
		{Key: "t", Value: &v8value.Value{Kind: v8value.KindNumber, Number: 1727280000}},
	}}
	return indexeddb.Record{Kind: webstore.KindIndexedDB, Namespace: "model-storage", Container: "message", Value: value}
}

// discoverWorld is a world whose IndexedDB holds two chat messages and
// whose model answers the two discovery questions.
func discoverWorld(t *testing.T, fieldsReply string) *fakeWorld {
	t.Helper()
	world := newFakeWorld()
	world.cfg.Sources.IndexedDBSchemaDir = t.TempDir()
	world.indexedDBRecords = []indexeddb.Record{chatRecord("A"), chatRecord("B")}
	world.generator.StructuredReplies = []string{discoverStoreReply, fieldsReply}
	return world
}

func TestIDBDiscoverSavesTheSchemaAndExplainsTheNextStep(t *testing.T) {
	world := discoverWorld(t, discoverFieldsReply)
	code, stdout, stderr := world.run("idb-discover", "--name", "whatsapp", whatsappDir)
	if code != 0 || !strings.Contains(stdout, `schema "whatsapp" salvo`) || !strings.Contains(stdout, "2 mensagens de 2 registros") {
		t.Fatalf("expected the saved schema reported, got %d:\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `{"whatsapp":["`+whatsappDir+`"]}`) || !strings.Contains(stdout, "cade ingest whatsapp") {
		t.Fatalf("expected the config hint and the ingest command, got:\n%s", stdout)
	}
	saved, err := os.ReadFile(filepath.Join(world.cfg.Sources.IndexedDBSchemaDir, "whatsapp.json"))
	if err != nil || !strings.Contains(string(saved), `"transform": "unix_s"`) {
		t.Fatalf("expected the schema file, got %q (err %v)", saved, err)
	}
	if !world.generator.Closed || len(world.generator.Grammars) != 2 {
		t.Fatalf("expected two grammar-bound calls and the model closed, got %d calls", len(world.generator.Grammars))
	}
}

func TestIDBDiscoverRefusesToReplaceWithoutForce(t *testing.T) {
	world := discoverWorld(t, discoverFieldsReply)
	world.run("idb-discover", "--name", "whatsapp", whatsappDir)
	world.generator.StructuredReplies = []string{discoverStoreReply, discoverFieldsReply}
	code, _, stderr := world.run("idb-discover", "--name", "whatsapp", whatsappDir)
	if code != 1 || !strings.Contains(stderr, "--force") {
		t.Fatalf("expected a refusal mentioning --force, got %d %q", code, stderr)
	}
	world.generator.StructuredReplies = []string{discoverStoreReply, discoverFieldsReply}
	if code, _, stderr := world.run("idb-discover", "--force", "--name", "whatsapp", whatsappDir); code != 0 {
		t.Fatalf("expected --force to replace, got %d %q", code, stderr)
	}
}

func TestIDBDiscoverPrintOnlyDoesNotSave(t *testing.T) {
	world := discoverWorld(t, discoverFieldsReply)
	code, stdout, _ := world.run("idb-discover", "--print", "--name", "whatsapp", "--source", "zap", whatsappDir)
	if code != 0 || !strings.Contains(stdout, `"source": "zap"`) {
		t.Fatalf("expected the schema printed with its source, got %d:\n%s", code, stdout)
	}
	if entries, _ := os.ReadDir(world.cfg.Sources.IndexedDBSchemaDir); len(entries) != 0 {
		t.Fatalf("--print must not save, found %d files", len(entries))
	}
}

func TestIDBDiscoverPrintsASchemaThatMapsNothing(t *testing.T) {
	world := discoverWorld(t, strings.Replace(discoverFieldsReply, `"keep": null`, `"keep": {"path": "$.body", "in": ["nada"]}`, 1))
	code, stdout, stderr := world.run("idb-discover", "--name", "whatsapp", whatsappDir)
	if code != 1 || !strings.Contains(stdout, `"name": "whatsapp"`) || !strings.Contains(stderr, "maps no message") {
		t.Fatalf("expected the schema printed for review and the error, got %d:\n%s%s", code, stdout, stderr)
	}
}

func TestIDBDiscoverRequiresNameAndDirectory(t *testing.T) {
	for _, args := range [][]string{{"idb-discover", whatsappDir}, {"idb-discover", "--name", "x"}} {
		if code, _, stderr := newFakeWorld().run(args...); code != 1 || !strings.Contains(stderr, "--name") {
			t.Errorf("%v: expected a usage error, got %d %q", args, code, stderr)
		}
	}
}

func TestIDBDiscoverReportsReadAndModelErrors(t *testing.T) {
	if code, _, stderr := newFakeWorld().run("idb-discover", "--name", "x", "/missing"); code != 1 || !strings.Contains(stderr, "no such directory") {
		t.Errorf("read error: got %d %q", code, stderr)
	}
	world := discoverWorld(t, discoverFieldsReply)
	world.generatorLoadError = errors.New("model file not found")
	if code, _, stderr := world.run("idb-discover", "--name", "x", whatsappDir); code != 1 || !strings.Contains(stderr, "model file not found") {
		t.Errorf("model error: got %d %q", code, stderr)
	}
}

func TestIDBDiscoverWritesTheKindOfTheStorageItDetected(t *testing.T) {
	world := discoverWorld(t, discoverFieldsReply)
	records := []webstore.Record{chatRecord("A"), chatRecord("B")}
	for i := range records {
		records[i].Kind = webstore.KindLocalStorage
	}
	world.otherStores = webstore.Readers{&testfakes.FakeStoreReader{StoreKind: webstore.KindLocalStorage, Suffix: "/ls", Records: records}}
	code, stdout, stderr := world.run("idb-discover", "--name", "chat", "/p/https_chat.example_0/ls")
	saved, err := os.ReadFile(filepath.Join(world.cfg.Sources.IndexedDBSchemaDir, "chat.json"))
	if code != 0 || err != nil || !strings.Contains(string(saved), `"kind": "local_storage"`) {
		t.Fatalf("expected a local_storage schema saved, got %d %q (err %v):\n%s%s", code, saved, err, stdout, stderr)
	}
}
