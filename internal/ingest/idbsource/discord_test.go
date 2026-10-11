package idbsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/chromiumcache"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
)

// The reviewed Discord schema, read by the real HTTP cache reader from a
// synthetic Cache_Data with the shape of the desktop app's: no code of
// Discord's own, only the schema and the configured URL pattern.
const (
	discordSchemaPath  = "../../../testdata/idb-schemas/discord.json"
	discordMessagesURL = "https://discordapp.com/api/v9/channels/1001/messages?limit=50"
	discordMessages    = `[
	{"type":0,"content":"deploy feito","id":"2002","channel_id":"1001","author":{"id":"3003","username":"ana.s","global_name":"Ana Souza"},"timestamp":"2026-09-30T14:05:00.123000+00:00","edited_timestamp":null},
	{"type":19,"content":"valeu!","id":"2003","channel_id":"1001","author":{"id":"3004","username":"bruno","global_name":null},"timestamp":"2026-09-30T14:06:00+00:00","edited_timestamp":"2026-09-30T14:07:00+00:00"},
	{"type":7,"content":"","id":"2004","channel_id":"1001","author":{"id":"3005","username":"carla","global_name":"Carla"},"timestamp":"2026-09-30T14:08:00+00:00"}
]`
)

func writeDiscordCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testcheck.NoError(t, os.Mkdir(filepath.Join(dir, "index-dir"), 0o755))
	entries := map[string]testfakes.SimpleCacheEntry{
		"0d1c857aa3989262_0": {Key: "1/0/" + discordMessagesURL, Stream0: []byte("HTTP/1.1 200\x00content-type:application/json\x00\x00"), Body: []byte(discordMessages)},
		"1111111111111111_0": {Key: "1/0/https://discordapp.com/api/v9/users/3003/profile", Stream0: []byte("HTTP/1.1 200\x00\x00"), Body: []byte(`{"bio":"x"}`)},
	}
	for name, entry := range entries {
		testcheck.NoError(t, os.WriteFile(filepath.Join(dir, name), entry.Bytes(), 0o600))
	}
	return dir
}

func collectDiscord(t *testing.T) map[string]event.Event {
	t.Helper()
	scope, err := requestcache.NewScope(map[string][]string{"discord-messages": {"https://discordapp.com/api/v*/channels/*/messages*"}})
	testcheck.NoError(t, err)
	readers := webstore.Readers{chromiumcache.NewHTTPCacheReader(scope)}
	collector, err := NewCollector(readers, writeDiscordCache(t), loadSchema(t, discordSchemaPath))
	testcheck.NoError(t, err)
	byID := map[string]event.Event{}
	err = collector.CollectEvents(context.Background(), func(ev event.Event) error {
		byID[ev.Message().MessageID] = ev
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return byID
}

func TestDiscordSchemaKeepsMessagesAndRepliesOnly(t *testing.T) {
	events := collectDiscord(t)
	if len(events) != 2 {
		t.Fatalf("expected the message and the reply, got %d: %v", len(events), events)
	}
	if _, joined := events["2004"]; joined {
		t.Fatal("a member-join system message must be left out")
	}
}

func TestDiscordSchemaMapsTheMessageFields(t *testing.T) {
	ev := collectDiscord(t)["2002"]
	message := ev.Message()
	if message.ConversationID != "1001" || message.Sender != "Ana Souza" || message.SenderMRI != "3003" || message.Text != "deploy feito" {
		t.Fatalf("unexpected message %+v", message)
	}
	if want := time.Date(2026, 9, 30, 14, 5, 0, 123000000, time.UTC); !ev.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %v; want %v", ev.Timestamp, want)
	}
	if ev.UID != event.StableID("discord", "1001", "2002") {
		t.Fatal("expected the UID keyed on the channel and the message id")
	}
}

func TestDiscordSchemaFallsBackToTheUsernameAndKeepsTheEdit(t *testing.T) {
	reply := collectDiscord(t)["2003"].Message()
	if reply.Sender != "bruno" || reply.Revision != "2026-09-30T14:07:00+00:00" {
		t.Fatalf("expected the username and the edit time, got %+v", reply)
	}
}
