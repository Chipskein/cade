package sqlitestore

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func authoredCommit(uid, author, email string) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime, Content: "commit " + uid,
		Metadata: event.Commit{Repository: "/src/api", Hash: uid, Author: author, Email: email}.Metadata()}
}

// Commits stored before identities were known get marked on the next run,
// and only changed marks are written.
func TestMarkCommitAuthorship(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, authoredCommit("a", "Ana", "ANA@x.io"), nil)
	mustSave(t, store, authoredCommit("b", "Rui Costa", "rui@x.io"), nil)
	changed, err := store.MarkCommitAuthorship(ctx, "/src/api", []string{"ana@x.io"})
	mine, _, _ := store.StoredEvent(ctx, "a")
	other, _, _ := store.StoredEvent(ctx, "b")
	if err != nil || changed != 2 || mine.Commit().Authorship != event.AuthorshipMine || !other.IsOthersCommit() {
		t.Fatalf("expected both marked, got %d changes, %q and %q (err %v)", changed, mine.Commit().Authorship, other.Commit().Authorship, err)
	}
	if again, _ := store.MarkCommitAuthorship(ctx, "/src/api", []string{"ana@x.io"}); again != 0 {
		t.Fatalf("expected no rewrite when nothing changed, got %d", again)
	}
	if other, _ := store.MarkCommitAuthorship(ctx, "/src/outro", []string{"ana@x.io"}); other != 0 {
		t.Fatalf("expected other repositories untouched, got %d", other)
	}
}
