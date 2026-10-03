package teamssource

import (
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

func TestParseMessageRequiresTimestamp(t *testing.T) {
	value := obj("id", str("1"), "conversationId", str("c"), "messageType", str("Text"), "content", str("oi"))
	if _, ok := parseMessage(value, nil); ok {
		t.Fatal("expected a message without arrival time to be rejected")
	}
}

func TestArrivalTimeFallsBackToClient(t *testing.T) {
	value := obj("clientArrivalTime", num(1000))
	if got := arrivalTime(value); got.UnixMilli() != 1000 {
		t.Fatalf("expected client arrival time, got %s", got)
	}
}

func TestSenderNameFallbacks(t *testing.T) {
	if senderName(obj("fromDisplayNameInToken", str("Bruno")), nil) != "Bruno" || senderName(obj(), nil) != "desconhecido" {
		t.Fatal("expected token name fallback, then desconhecido")
	}
}

func TestIsDeleted(t *testing.T) {
	undefined := &v8value.Value{Kind: v8value.KindUndefined}
	if isDeleted(obj("deletionInfo", undefined)) || !isDeleted(obj("deletionInfo", obj())) {
		t.Fatal("expected only a deletionInfo object to mean deleted")
	}
}

func TestDescribeConversationKinds(t *testing.T) {
	cases := map[string]conversationInfo{
		"Chat":    {kind: event.KindChat, title: "Ana, Bruno"},
		"Meeting": {kind: event.KindMeeting, title: "Daily"},
		"Space":   {kind: event.KindChannel, title: "Atlas › Geral"},
		"Other":   {kind: event.KindOther},
	}
	values := map[string]*v8value.Value{
		"Chat":    obj("type", str("Chat"), "chatTitle", obj("longTitle", str("Ana, Bruno"))),
		"Meeting": obj("type", str("Meeting"), "threadProperties", obj("topic", str("Daily"))),
		"Space":   obj("type", str("Space"), "threadProperties", obj("spaceThreadTopic", str("Atlas"))),
		"Other":   obj("type", str("Thread")),
	}
	for name, expected := range cases {
		if got := describeConversation(values[name], nil); got != expected {
			t.Errorf("%s: expected %+v, got %+v", name, expected, got)
		}
	}
}

func TestChannelTitleWithoutTeam(t *testing.T) {
	if channelTitle("", "Avisos") != "Avisos" || channelTitle("Time", "") != "Time › Geral" {
		t.Fatal("unexpected channel titles")
	}
}

func TestProfileNames(t *testing.T) {
	record := indexeddb.Record{Database: "Teams:profiles:x", Store: "profiles", Value: obj("mri", str("8:orgid:a"), "displayName", str("Ana"))}
	if names := profileNames([]indexeddb.Record{record}); names["8:orgid:a"] != "Ana" {
		t.Fatalf("unexpected profile names %v", names)
	}
}

func TestConversationTitleFromParticipants(t *testing.T) {
	users := &v8value.Value{Kind: v8value.KindArray, Items: []*v8value.Value{
		obj("displayName", str("Ana")), nil, obj("displayName", str("Bruno")),
	}}
	conversation := obj("id", str("c1"), "type", str("Chat"), "chatTitle", obj("avatarUsersInfo", users))
	infos := conversationInfos([]indexeddb.Record{{Database: testConversationDB, Store: conversationStore, Value: conversation}})
	if infos["c1"].title != "Ana, Bruno" {
		t.Fatalf("expected participant names, got %q", infos["c1"].title)
	}
}

func TestParticipantNamesCapsList(t *testing.T) {
	users := &v8value.Value{Kind: v8value.KindArray}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		users.Items = append(users.Items, obj("displayName", str(name)))
	}
	if got := participantNames(users); got != "a, b, c, d" {
		t.Fatalf("expected four names, got %q", got)
	}
}
