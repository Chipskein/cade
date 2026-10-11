package requestcache

import (
	"strings"
	"testing"
)

const (
	discordMessages = "https://discord.com/api/v*/channels/*/messages*"
	desktopMessages = "https://discordapp.com/api/v*/channels/*/messages*"
)

func discordScope(t *testing.T) Scope {
	t.Helper()
	scope, err := NewScope(map[string][]string{"discord-messages": {discordMessages, desktopMessages}})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return scope
}

func TestMatchNamesThePatternOfTheURL(t *testing.T) {
	scope := discordScope(t)
	for _, url := range []string{
		"https://discord.com/api/v9/channels/1151677626931490828/messages?limit=50",
		"https://discordapp.com/api/v10/channels/42/messages",
	} {
		if name, ok := scope.Match(url); !ok || name != "discord-messages" {
			t.Errorf("Match(%q) = %q, %v; want discord-messages", url, name, ok)
		}
	}
}

func TestMatchLeavesOutEveryOtherURL(t *testing.T) {
	scope := discordScope(t)
	for _, url := range []string{
		"https://discord.com/api/v9/users/@me/profile",
		"https://discord.com.evil.test/api/v9/channels/1/messages",
		"http://discord.com/api/v9/channels/1/messages",
		"https://github.com/api/v9/channels/1/messages",
		"https://discord.com/api/v9/channels/1/message",
	} {
		if name, ok := scope.Match(url); ok {
			t.Errorf("Match(%q) = %q; want no match", url, name)
		}
	}
}

func TestMatchWithoutWildcardIsExact(t *testing.T) {
	scope, err := NewScope(map[string][]string{"inbox": {"https://mail.test/inbox"}})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	if _, ok := scope.Match("https://mail.test/inbox"); !ok {
		t.Fatal("Match(exact URL) = false")
	}
	if _, ok := scope.Match("https://mail.test/inbox?page=2"); ok {
		t.Fatal("Match(longer URL) = true; want only the exact URL")
	}
}

func TestMatchPicksTheSameNameForAnOverlap(t *testing.T) {
	both := map[string][]string{"b-all": {"https://chat.test/*"}, "a-messages": {"https://chat.test/messages*"}}
	for range 5 {
		scope, err := NewScope(both)
		if err != nil {
			t.Fatalf("NewScope: %v", err)
		}
		if name, _ := scope.Match("https://chat.test/messages"); name != "a-messages" {
			t.Fatalf("Match = %q; want a-messages, the first name in order", name)
		}
	}
}

func TestNewScopeRefusesAWildcardInTheOrigin(t *testing.T) {
	for _, pattern := range []string{"https://*.discord.com/api/*", "https://discord.com*", "*", "discord.com/api/*", "https:///api/*"} {
		_, err := NewScope(map[string][]string{"chat": {pattern}})
		if err == nil || !strings.Contains(err.Error(), pattern) {
			t.Errorf("NewScope(%q) error = %v; want the pattern refused by value", pattern, err)
		}
	}
}

func TestNewScopeRefusesABadName(t *testing.T) {
	_, err := NewScope(map[string][]string{"Discord Messages": {discordMessages}})
	if err == nil || !strings.Contains(err.Error(), `"Discord Messages"`) {
		t.Fatalf("NewScope error = %v; want the name refused by value", err)
	}
}

func TestEmptyScopeMatchesNothing(t *testing.T) {
	scope, err := NewScope(nil)
	if err != nil || !scope.Empty() {
		t.Fatalf("NewScope(nil) = %+v, %v; want an empty scope", scope, err)
	}
	if _, ok := scope.Match("https://discord.com/api/v9/channels/1/messages"); ok {
		t.Fatal("empty scope matched a URL")
	}
}
