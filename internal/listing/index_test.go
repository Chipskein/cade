package listing

import (
	"errors"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestPeopleOfIndexesSenderConversationAndMentions(t *testing.T) {
	ev := groupMessage("Ana Prado - Atlas", "@Marcos Lima e @Vitor olhem", false)
	want := []PersonEntry{
		{Role: RoleSender, Name: "ana prado atlas", Key: "ana prado atlas"},
		{Role: RoleConversation, Name: "interno", Key: "interno"},
		{Role: RoleMentioned, Name: "marcos", Key: "marcos"},
		{Role: RoleMentioned, Name: "vitor", Key: "vitor"},
	}
	if got := PeopleOf(ev); !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestPeopleOfUsesCommitAuthorAndSkipsNamesWithoutLetters(t *testing.T) {
	commit := event.Event{Source: event.SourceGit, Content: "@Rui fix", Metadata: event.Commit{Author: "Willian Costa"}.Metadata()}
	want := []PersonEntry{{Role: RoleAuthor, Name: "willian costa", Key: "wilian costa"}}
	if got := PeopleOf(commit); !slices.Equal(got, want) {
		t.Fatalf("expected only the author (no mentions outside Teams), got %+v", got)
	}
	if got := PeopleOf(message("123", "", "false", "chat")); len(got) != 0 {
		t.Fatalf("expected no entry for a name without letters, got %+v", got)
	}
}

func TestIndexedDirection(t *testing.T) {
	cases := map[string]event.Event{
		"sent": message("Eu", "", "true", "chat"), "received": message("Ana", "", "false", "chat"),
		"channel": message("Ana", "", "false", "canal"), "": {Source: event.SourceGit},
	}
	for want, ev := range cases {
		if got := IndexedDirection(ev); got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	}
	if Sent.Indexed() != "sent" || Received.Indexed() != "received" || AnyDirection.Indexed() != "" {
		t.Fatal("unexpected stored value per direction")
	}
}

func TestResolveReturnsTheProbeError(t *testing.T) {
	failure := errors.New("database locked")
	_, err := Criteria{People: []string{"Ana"}}.Resolve(func(PersonMatcher) (bool, error) { return false, failure })
	if !errors.Is(err, failure) {
		t.Fatalf("expected the probe error, got %v", err)
	}
}

func TestResolveFallsBackToFirstName(t *testing.T) {
	asked := []string{}
	resolution, _ := Criteria{People: []string{"Ana Souza"}}.Resolve(func(p PersonMatcher) (bool, error) {
		asked = append(asked, p.Name)
		return p.Name == "ana", nil
	})
	if !slices.Equal(asked, []string{"ana souza", "ana"}) || !slices.Equal(resolution.Matched, []string{"ana"}) {
		t.Fatalf("expected the full name then the first word, asked %v, matched %v", asked, resolution.Matched)
	}
}

func TestSelectMatchesApplyWithResolvedPeople(t *testing.T) {
	criteria := Criteria{Direction: Received, People: []string{"Ana"}}
	want, _, _ := criteria.Apply(dayOfMessages)
	got := Select(dayOfMessages, Received, []PersonMatcher{matcherFor("ana", Received)})
	if senders(got) != senders(want) {
		t.Fatalf("expected Select to keep what Apply keeps, got %s want %s", senders(got), senders(want))
	}
}

func TestMatcherRoles(t *testing.T) {
	if roles := matcherFor("ana", Sent).Roles(); !slices.Equal(roles, []Role{RoleConversation}) {
		t.Fatalf("expected a sent-to person looked up in the conversation only, got %v", roles)
	}
	if roles := matcherFor("ana", AnyDirection).Roles(); len(roles) != 3 {
		t.Fatalf("expected sender, author and conversation, got %v", roles)
	}
}
