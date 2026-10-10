package entity

import (
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestRelationsOfCommitLinkTheAuthorToEachFile(t *testing.T) {
	ev := event.Event{Source: event.SourceGit, Metadata: event.Commit{Repository: "/src/api", Author: "Carla Dias",
		Files: []string{"erp/retry.go", "erp/backoff.go"}}.Metadata()}
	carla := Ref{Type: TypePerson, Key: "carla dias", Name: "carla dias", Role: RoleAuthor}
	want := []RelationRef{
		{From: carla, Relation: RelationChanged, To: Ref{Type: TypeFile, Key: "/src/api/erp/retry.go", Name: "retry.go", Role: RoleChanged}},
		{From: carla, Relation: RelationChanged, To: Ref{Type: TypeFile, Key: "/src/api/erp/backoff.go", Name: "backoff.go", Role: RoleChanged}},
	}
	if got := RelationsOf(ev); !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestRelationsOfEventWithoutAuthorAreNone(t *testing.T) {
	message := event.Event{Source: event.SourceTeams, Content: "Ana Souza: oi", Metadata: event.Message{Sender: "Ana Souza"}.Metadata()}
	commit := event.Event{Source: event.SourceGit, Metadata: event.Commit{Repository: "/src/api", Files: []string{"a.go"}}.Metadata()}
	if got := append(RelationsOf(message), RelationsOf(commit)...); len(got) != 0 {
		t.Fatalf("expected no relations without a commit author, got %+v", got)
	}
}
