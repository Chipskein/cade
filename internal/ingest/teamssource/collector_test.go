package teamssource

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

const (
	testReplyChainDB   = "Teams:replychain-manager:react-web-client:t:u:en-us"
	testConversationDB = "Teams:conversation-manager:react-web-client:t:u:en-us"
	testConversationID = "19:chat@thread.v2"
)

var sentAt = time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)

// FakeIndexedDBReader returns canned records, or FailWith.
type FakeIndexedDBReader struct {
	Records  []indexeddb.Record
	FailWith error
}

func (f FakeIndexedDBReader) Read(string) ([]indexeddb.Record, error) {
	return f.Records, f.FailWith
}

func str(text string) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindString, Text: text}
}

func num(number float64) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindNumber, Number: number}
}

func obj(pairs ...any) *v8value.Value {
	value := &v8value.Value{Kind: v8value.KindObject}
	for i := 0; i < len(pairs); i += 2 {
		value.Properties = append(value.Properties, v8value.Property{Key: pairs[i].(string), Value: pairs[i+1].(*v8value.Value)})
	}
	return value
}

func message(id, messageType, content string) *v8value.Value {
	return obj("id", str(id), "conversationId", str(testConversationID), "messageType", str(messageType),
		"content", str(content), "imDisplayName", str("Ana Souza"), "creator", str("8:orgid:ana"),
		"originalArrivalTime", num(float64(sentAt.UnixMilli())), "isSentByCurrentUser", &v8value.Value{Kind: v8value.KindBool})
}

func replyChain(messages ...*v8value.Value) indexeddb.Record {
	messageMap := &v8value.Value{Kind: v8value.KindObject}
	for _, m := range messages {
		messageMap.Properties = append(messageMap.Properties, v8value.Property{Key: m.Get("id").String(), Value: m})
	}
	return indexeddb.Record{Database: testReplyChainDB, Store: replyChainStore, Value: obj("messageMap", messageMap)}
}

func conversationRecord() indexeddb.Record {
	conversation := obj("id", str(testConversationID), "type", str("Chat"), "threadProperties", obj("topic", str("Release 2.0")))
	return indexeddb.Record{Database: testConversationDB, Store: conversationStore, Value: conversation}
}

func collectTeams(t *testing.T, reader FakeIndexedDBReader) ([]event.Event, error) {
	t.Helper()
	var events []event.Event
	err := NewCollector(reader.Read, "/p/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb").CollectEvents(context.Background(),
		func(ev event.Event) error { events = append(events, ev); return nil })
	return events, err
}

func TestCollectEmitsConversationMessages(t *testing.T) {
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{conversationRecord(), replyChain(
		message("1", "RichText/Html", "<p>Deploy <b>amanhã</b> às 10h</p>"),
		message("2", "ThreadActivity/AddMember", "<addmember/>"),
		message("3", "Event/Call", "<partlist/>"),
	)}}
	events, err := collectTeams(t, reader)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected only the chat message, got %d (err %v)", len(events), err)
	}
	ev := events[0]
	if ev.Content != "Ana Souza: Deploy amanhã às 10h\nConversa: chat Release 2.0\nRecebida por você" || !ev.Timestamp.Equal(sentAt) || ev.Source != event.SourceTeams {
		t.Fatalf("unexpected event %+v", ev)
	}
	if ev.Metadata["conversation"] != "Release 2.0" || ev.Metadata["conversation_kind"] != "chat" || ev.Metadata["origin"] != "https_teams.cloud.microsoft_0" || ev.Metadata["sent_by_me"] != "false" {
		t.Fatalf("unexpected metadata %+v", ev.Metadata)
	}
}

func TestCollectUIDIsStableAcrossOrigins(t *testing.T) {
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(message("1", "Text", "oi"))}})
	if events[0].UID != event.StableID(event.SourceTeams, testConversationID, "1") {
		t.Fatalf("expected UID from conversation and message id, got %q", events[0].UID)
	}
}

func TestCollectSkipsDeletedAndEmptyMessages(t *testing.T) {
	deleted := message("1", "Text", "segredo")
	deleted.Properties = append(deleted.Properties, v8value.Property{Key: "deletionInfo", Value: obj()})
	empty := message("2", "RichText/Html", "<p> </p>")
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(deleted, empty)}})
	if len(events) != 0 {
		t.Fatalf("expected no events, got %+v", events)
	}
}

func TestCollectIgnoresOtherStores(t *testing.T) {
	pinned := replyChain(message("1", "Text", "oi"))
	pinned.Store = "pinned-messages-store"
	failed := indexeddb.Record{Database: testReplyChainDB, Store: replyChainStore, DecodeErr: errors.New("bad")}
	valid := replyChain(message("2", "Text", "tchau"))
	events, err := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{pinned, failed, valid}})
	if err != nil || len(events) != 1 || !strings.Contains(events[0].Content, "tchau") {
		t.Fatalf("expected only the reply chain's message, got %d (err %v)", len(events), err)
	}
}

// Regression guard: a Teams update renaming the message store made
// ingestion report "0 novos" silently.
func TestCollectFailsWhenMessageStoreIsMissing(t *testing.T) {
	renamed := replyChain(message("1", "Text", "oi"))
	renamed.Store = "replychains-v2"
	_, err := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{conversationRecord(), renamed}})
	if err == nil || !strings.Contains(err.Error(), `none in store "replychains"`) || !strings.Contains(err.Error(), "cade teams-schema") {
		t.Fatalf("expected an unrecognized-format error pointing to teams-schema, got %v", err)
	}
}

func TestCollectFailsWhenNoMessageHasTheExpectedFields(t *testing.T) {
	renamed := obj("id", str("1"), "conversationId", str(testConversationID), "messageType", str("Text"),
		"body", str("oi"), "originalArrivalTime", num(float64(sentAt.UnixMilli())))
	_, err := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(renamed)}})
	if err == nil || !strings.Contains(err.Error(), "1 cached messages and none with the fields") {
		t.Fatalf("expected an unrecognized-fields error, got %v", err)
	}
}

// System and deleted messages are skipped on purpose, not an unknown format.
func TestCollectAcceptsCacheWithOnlySkippedMessages(t *testing.T) {
	deleted := message("1", "Text", "segredo")
	deleted.Properties = append(deleted.Properties, v8value.Property{Key: "deletionInfo", Value: obj()})
	records := []indexeddb.Record{replyChain(deleted, message("2", "ThreadActivity/AddMember", "<addmember/>"))}
	if events, err := collectTeams(t, FakeIndexedDBReader{Records: records}); err != nil || len(events) != 0 {
		t.Fatalf("expected no events and no error, got %d (err %v)", len(events), err)
	}
}

func TestCollectAcceptsEmptyIndexedDB(t *testing.T) {
	if _, err := collectTeams(t, FakeIndexedDBReader{}); err != nil {
		t.Fatalf("an empty origin is not a format change, got %v", err)
	}
}

func TestCollectWrapsReadError(t *testing.T) {
	_, err := collectTeams(t, FakeIndexedDBReader{FailWith: errors.New("locked")})
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
}

func TestCollectStopsWhenEmitFails(t *testing.T) {
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(message("1", "Text", "a"), message("2", "Text", "b"))}}
	calls := 0
	err := NewCollector(reader.Read, "/d").CollectEvents(context.Background(), func(event.Event) error {
		calls++
		return errors.New("disk full")
	})
	if err == nil || calls != 1 {
		t.Fatalf("expected to stop after the first failure, got %d calls (err %v)", calls, err)
	}
}

func TestCollectHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(message("1", "Text", "a"))}}
	if err := NewCollector(reader.Read, "/d").CollectEvents(ctx, func(event.Event) error { return nil }); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestOriginName(t *testing.T) {
	if got := originName("/x/https_teams.microsoft.com_0.indexeddb.leveldb/"); got != "https_teams.microsoft.com_0" {
		t.Fatalf("unexpected origin %q", got)
	}
	if got := originName("/p/storage/default/https+++teams.microsoft.com/idb"); got != "https+++teams.microsoft.com" {
		t.Fatalf("expected the Firefox origin directory, got %q", got)
	}
}

// Regression: the activity feed re-posts channel messages, which showed up
// as duplicate "messages I received" from an unknown sender.
func TestCollectSkipsNotificationStreams(t *testing.T) {
	notification := message("9", "RichText/Html", "aviso")
	notification.Properties = append(notification.Properties, v8value.Property{Key: "threadType", Value: str("streamofnotifications")})
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(notification)}})
	if len(events) != 0 {
		t.Fatalf("expected notification stream copies to be skipped, got %d", len(events))
	}
}

func TestCollectMarksChannelPosts(t *testing.T) {
	team := obj("id", str("19:team@thread.tacv2"), "type", str("Space"), "threadProperties", obj("spaceThreadTopic", str("Atlas")))
	channel := obj("id", str(testConversationID), "type", str("Topic"), "teamId", str("19:team@thread.tacv2"),
		"threadProperties", obj("topic", str("Avisos")))
	records := []indexeddb.Record{
		{Database: testConversationDB, Store: conversationStore, Value: team},
		{Database: testConversationDB, Store: conversationStore, Value: channel},
		replyChain(message("1", "Text", "antecipem os apontamentos")),
	}
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: records})
	expected := "Ana Souza: antecipem os apontamentos\nConversa: canal Atlas › Avisos\nPublicada no canal (não enviada diretamente a você)"
	if len(events) != 1 || events[0].Content != expected {
		t.Fatalf("expected a channel post, got %+v", events)
	}
}

func TestCollectResolvesSenderFromProfiles(t *testing.T) {
	anonymous := obj("id", str("1"), "conversationId", str(testConversationID), "messageType", str("Text"),
		"content", str("oi"), "creator", str("8:orgid:ana"), "originalArrivalTime", num(float64(sentAt.UnixMilli())))
	profile := indexeddb.Record{Database: "Teams:profiles:react-web-client:t:u:en-us", Store: "profiles",
		Value: obj("mri", str("8:orgid:ana"), "displayName", str("Ana Prado"))}
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{profile, replyChain(anonymous)}})
	if len(events) != 1 || events[0].Metadata["sender"] != "Ana Prado" {
		t.Fatalf("expected the sender name from profiles, got %+v", events)
	}
}

func TestCollectCarriesVersionAsRevision(t *testing.T) {
	edited := message("1", "Text", "deploy às 19h")
	edited.Properties = append(edited.Properties, v8value.Property{Key: "version", Value: str("1758800000123")})
	events, _ := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(edited)}})
	if revision, ok := events[0].Revision(); !ok || revision != 1758800000123 {
		t.Fatalf("expected the message version as revision, got %v %v", revision, ok)
	}
}

// Regression: a reply chain without messageMap panicked with a nil pointer
// (found by the format sample test).
func TestReplyChainWithoutMessageMapIsSkipped(t *testing.T) {
	bare := indexeddb.Record{Database: testReplyChainDB, Store: replyChainStore, Value: obj("id", str("19:big@thread.v2"))}
	events, err := collectTeams(t, FakeIndexedDBReader{Records: []indexeddb.Record{bare, replyChain(message("1", "Text", "oi"))}})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected the other chain's message and no error, got %d events, %v", len(events), err)
	}
}
