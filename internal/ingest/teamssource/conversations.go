package teamssource

import (
	"strings"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// Database-name prefixes of the Teams web client's IndexedDB managers; the
// suffix carries tenant and user ids, which vary per account.
const (
	replyChainDatabasePrefix   = "Teams:replychain-manager:"
	replyChainStore            = "replychains"
	conversationDatabasePrefix = "Teams:conversation-manager:"
	conversationStore          = "conversations"
	maxTitleParticipants       = 4
	defaultChannelName         = "Geral"
)

// conversationKind says who a message was addressed to, which is what
// separates "messages I received" from announcements posted to a team.
type conversationKind string

const (
	kindChat    conversationKind = "chat"
	kindChannel conversationKind = "canal"
	kindMeeting conversationKind = "reunião"
	kindOther   conversationKind = "conversa"
)

// conversationInfo is what an event shows about its conversation.
type conversationInfo struct {
	kind  conversationKind
	title string
}

func isStore(record indexeddb.Record, databasePrefix, store string) bool {
	return record.DecodeErr == nil && record.Store == store && strings.HasPrefix(record.Database, databasePrefix)
}

// conversationInfos maps conversation ids to their kind and readable name.
// Channels are named "Team › Channel", so the team (Space) names are
// collected first.
func conversationInfos(records []indexeddb.Record) map[string]conversationInfo {
	conversations := conversationValues(records)
	teams := teamNames(conversations)
	infos := make(map[string]conversationInfo, len(conversations))
	for id, conversation := range conversations {
		infos[id] = describeConversation(conversation, teams)
	}
	return infos
}

func conversationValues(records []indexeddb.Record) map[string]*v8value.Value {
	conversations := map[string]*v8value.Value{}
	for _, record := range records {
		if isStore(record, conversationDatabasePrefix, conversationStore) {
			conversations[record.Value.Get("id").String()] = record.Value
		}
	}
	return conversations
}

// teamNames maps team ids to names. A team's General channel shares the
// team's id and carries the team name in spaceThreadTopic.
func teamNames(conversations map[string]*v8value.Value) map[string]string {
	teams := map[string]string{}
	for id, conversation := range conversations {
		if name := threadProperty(conversation, "spaceThreadTopic"); name != "" {
			teams[id] = name
		}
	}
	return teams
}

func describeConversation(conversation *v8value.Value, teams map[string]string) conversationInfo {
	switch conversation.Get("type").String() {
	case "Topic":
		return conversationInfo{kind: kindChannel, title: channelTitle(teams[conversation.Get("teamId").String()], threadProperty(conversation, "topic"))}
	case "Space":
		return conversationInfo{kind: kindChannel, title: channelTitle(threadProperty(conversation, "spaceThreadTopic"), threadProperty(conversation, "topic"))}
	case "Meeting":
		return conversationInfo{kind: kindMeeting, title: firstNonEmpty(threadProperty(conversation, "topic"), longTitle(conversation))}
	case "Chat", "StreamOfNotes":
		return conversationInfo{kind: kindChat, title: chatTitle(conversation)}
	}
	return conversationInfo{kind: kindOther, title: threadProperty(conversation, "topic")}
}

// channelTitle joins team and channel; an unnamed channel is the team's
// General channel.
func channelTitle(team, channel string) string {
	channel = firstNonEmpty(channel, defaultChannelName)
	if team == "" {
		return channel
	}
	return team + " › " + channel
}

func chatTitle(conversation *v8value.Value) string {
	return firstNonEmpty(threadProperty(conversation, "topic"), longTitle(conversation),
		participantNames(conversation.Get("chatTitle").Get("avatarUsersInfo")))
}

func threadProperty(conversation *v8value.Value, name string) string {
	return strings.TrimSpace(conversation.Get("threadProperties").Get(name).String())
}

func longTitle(conversation *v8value.Value) string {
	return strings.TrimSpace(conversation.Get("chatTitle").Get("longTitle").String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func participantNames(users *v8value.Value) string {
	if users == nil {
		return ""
	}
	var names []string
	for _, user := range users.Items {
		name := strings.TrimSpace(user.Get("displayName").String())
		if name != "" && len(names) < maxTitleParticipants {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}
