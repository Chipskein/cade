package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// worldIndexedDB is the fake IndexedDB reader of a world: it recognizes
// every location and holds one Teams-like record whose content must never
// appear in the output, or the world's indexedDBRecords.
type worldIndexedDB struct {
	world *fakeWorld
}

func (worldIndexedDB) Kind() webstore.Kind { return webstore.KindIndexedDB }

func (worldIndexedDB) Recognizes(string) bool { return true }

func (r worldIndexedDB) Read(dir string) ([]webstore.Record, error) {
	if strings.Contains(dir, "missing") {
		return nil, errors.New("no such directory")
	}
	if r.world.indexedDBRecords != nil {
		return r.world.indexedDBRecords, nil
	}
	message := &v8value.Value{Kind: v8value.KindObject, Properties: []v8value.Property{
		{Key: "content", Value: &v8value.Value{Kind: v8value.KindString, Text: "conteúdo privado"}},
	}}
	return []webstore.Record{{Kind: webstore.KindIndexedDB, Namespace: "Teams:replychain-manager", Container: "replychains", Value: message}}, nil
}

func TestTeamsSchemaPrintsStructureOnly(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("teams-schema", "/home/me/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb/")
	if code != 0 || !strings.Contains(stdout, `store "replychains": 1 registro (0 falhas, 0 em blob)`) || !strings.Contains(stdout, "string                 $.content") {
		t.Fatalf("expected the store summary, got %d:\n%s", code, stdout)
	}
	if strings.Contains(stdout, "conteúdo privado") || strings.Contains(stdout, "/home/me") {
		t.Fatalf("output leaked a value or the home path:\n%s", stdout)
	}
}

func TestTeamsSchemaNamesFirefoxOrigins(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("teams-schema", "/home/me/.floorp/p/storage/default/https+++web.whatsapp.com/idb")
	if code != 0 || !strings.HasPrefix(stdout, "# https+++web.whatsapp.com\n") {
		t.Fatalf("expected the origin as header, got %d:\n%s", code, stdout)
	}
}

func TestTeamsSchemaRequiresDirectory(t *testing.T) {
	if code, _, _ := newFakeWorld().run("teams-schema"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestTeamsSchemaReportsReadError(t *testing.T) {
	code, _, stderr := newFakeWorld().run("teams-schema", "/missing")
	if code != 1 || !strings.Contains(stderr, "no such directory") {
		t.Fatalf("expected the read error, got %d %q", code, stderr)
	}
}

func TestMostFrequentFieldsKeepsTopInPathOrder(t *testing.T) {
	fields := []idbschema.FieldStat{{Path: "$.a", Count: 1}, {Path: "$.b", Count: 9}, {Path: "$.c", Count: 5}}
	top := mostFrequentFields(fields, 2)
	if len(top) != 2 || top[0].Path != "$.b" || top[1].Path != "$.c" {
		t.Fatalf("expected [$.b $.c], got %+v", top)
	}
}

func TestDescribeKinds(t *testing.T) {
	kinds := map[v8value.Kind]int{v8value.KindString: 2, v8value.KindNull: 1}
	if got := describeKinds(kinds); got != "null|string" {
		t.Fatalf("expected null|string, got %q", got)
	}
}
