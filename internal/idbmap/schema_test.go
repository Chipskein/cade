package idbmap

import (
	"strings"
	"testing"
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
	if schema.Fields[FieldSentAt].Transform != TransformUnixS || schema.Fields[FieldSender].Lookup.Store != "contact" {
		t.Fatalf("unexpected schema %+v", schema)
	}
}

func TestParseRejectsInvalidSchemas(t *testing.T) {
	cases := map[string]struct{ old, new, mention string }{
		"newer version":       {`"version": 1`, `"version": 2`, "version 2"},
		"other target":        {`"target": "message/1"`, `"target": "document/1"`, "document/1"},
		"name with spaces":    {`"name": "whatsapp"`, `"name": "Whats App"`, "Whats App"},
		"no store":            {`"store": "message"}`, `"store": ""}`, "records.store"},
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
