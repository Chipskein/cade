package idbmap

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/webstore"
)

// metadataOnlySchema is the shape a WhatsApp Web schema takes: the body is
// encrypted, so there is no text field.
const metadataOnlySchema = `{
  "version": 1, "target": "message/1", "name": "whatsapp", "source": "whatsapp", "revision": 1,
  "records": {"database_prefix": "model-storage", "store": "message"},
  "require": [{"path": "$.type", "in": ["chat"]}, {"path": "$.msgRowOpaqueData", "kind_not": "undefined"}],
  "fields": {
    "message_id": {"paths": ["$.id"]},
    "conversation_id": {"paths": ["$.from"]},
    "sender": {"paths": ["$.author._serialized"],
      "lookup": {"database_prefix": "model-storage", "store": "contact", "key_path": "$.author._serialized", "match": "$.id", "values": ["$.name"]},
      "default": "desconhecido"},
    "sent_at": {"paths": ["$.t"], "transform": "unix_s"}
  }
}`

func TestParseAcceptsAMetadataOnlySchema(t *testing.T) {
	schema, err := Parse([]byte(metadataOnlySchema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if schema.Fields[FieldSentAt].Transform != TransformUnixS || schema.Fields[FieldSender].Lookup.Container != "contact" {
		t.Fatalf("unexpected schema %+v", schema)
	}
}

func TestParseRejectsInvalidSchemas(t *testing.T) {
	cases := map[string]struct{ old, new, mention string }{
		"newer version":       {`"version": 1`, `"version": 3`, "version 3"},
		"other target":        {`"target": "message/1"`, `"target": "document/1"`, "document/1"},
		"name with spaces":    {`"name": "whatsapp"`, `"name": "Whats App"`, "Whats App"},
		"no store":            {`"store": "message"}`, `"store": ""}`, "records.container"},
		"missing sent_at":     {`"sent_at"`, `"text"`, `"sent_at" is missing`},
		"unknown field":       {`"sent_at": {`, `"subject": {"paths": ["$.x"]}, "sent_at": {`, "unknown field"},
		"time transform":      {`"transform": "unix_s"`, `"transform": "html_text"`, "html_text"},
		"relative path":       {`["$.from"]`, `["from"]`, `"from"`},
		"unknown kind":        {`"kind_not": "undefined"`, `"kind_not": "indefinido"`, "indefinido"},
		"incomplete lookup":   {`"match": "$.id", `, ``, "lookup"},
		"typo in a key":       {`"default": "desconhecido"`, `"defualt": "desconhecido"`, "defualt"},
		"field with no rules": {`{"paths": ["$.id"]}`, `{}`, "no paths, lookup or default"},
	}
	for name, tc := range cases {
		raw := strings.Replace(metadataOnlySchema, tc.old, tc.new, 1)
		_, err := Parse([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), tc.mention) {
			t.Errorf("%s: expected an error mentioning %q, got %v", name, tc.mention, err)
		}
	}
}

func TestParseRejectsBrokenJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"version": 1,`)); err == nil {
		t.Fatal("expected an error for truncated JSON")
	}
}

func TestParseReadsAVersion1SchemaAsIndexedDB(t *testing.T) {
	schema, err := Parse([]byte(metadataOnlySchema))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := RecordSelector{Kind: webstore.KindIndexedDB, Location: Location{NamespacePrefix: "model-storage", Container: "message"}}
	if schema.Version != FormatVersion || schema.Records != want {
		t.Fatalf("version %d, records %+v; want version %d and %+v", schema.Version, schema.Records, FormatVersion, want)
	}
	if lookup := schema.Fields[FieldSender].Lookup.Location; lookup != (Location{NamespacePrefix: "model-storage", Container: "contact"}) {
		t.Fatalf("lookup location %+v; want the version 1 names moved", lookup)
	}
}

func TestEncodeWritesAVersion1SchemaInTheGenericNames(t *testing.T) {
	schema, _ := Parse([]byte(metadataOnlySchema))
	encoded, err := EncodeSchema(schema)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	text := string(encoded)
	for _, want := range []string{`"version": 2`, `"kind": "indexeddb"`, `"namespace_prefix": "model-storage"`, `"container": "contact"`} {
		if !strings.Contains(text, want) || strings.Contains(text, "database_prefix") {
			t.Fatalf("encoded schema lacks %s or keeps database_prefix:\n%s", want, text)
		}
	}
	if again, err := Parse(encoded); err != nil || !reflect.DeepEqual(again, schema) {
		t.Fatalf("reparse = %+v, %v; want the same schema", again, err)
	}
}

// localStorageSchema reads a chat kept as one localStorage key per
// conversation, with no namespace.
const localStorageSchema = `{
  "version": 2, "target": "message/1", "name": "chat", "source": "chat", "revision": 1,
  "records": {"kind": "local_storage", "container": "chat:history", "each": "$.messages[]"},
  "fields": {"message_id": {"paths": ["$.id"]}, "conversation_id": {"paths": ["$.room"]}, "sent_at": {"paths": ["$.at"], "transform": "iso8601"}}
}`

func TestParseReadsAVersion2SchemaOfAnotherStorage(t *testing.T) {
	schema, err := Parse([]byte(localStorageSchema))
	if err != nil || schema.Records.Kind != webstore.KindLocalStorage || schema.Records.Container != "chat:history" {
		t.Fatalf("parse = %+v, %v; want a local_storage schema", schema.Records, err)
	}
}

func TestParseRejectsMixedVersionNames(t *testing.T) {
	cases := map[string]struct{ raw, mention string }{
		"version 1 names in version 2": {strings.Replace(localStorageSchema, `"container"`, `"store"`, 1), "version 1 names"},
		"version 2 names in version 1": {strings.Replace(metadataOnlySchema, `"store": "message"`, `"container": "message"`, 1), "expected database_prefix and store"},
		"kind in version 1":            {strings.Replace(metadataOnlySchema, `"store": "message"`, `"store": "message", "kind": "indexeddb"`, 1), "records.kind"},
		"unknown kind":                 {strings.Replace(localStorageSchema, `"local_storage"`, `"local-storage"`, 1), "local-storage"},
	}
	for name, tc := range cases {
		if _, err := Parse([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), tc.mention) {
			t.Errorf("%s: expected an error mentioning %q, got %v", name, tc.mention, err)
		}
	}
}
