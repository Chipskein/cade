package event

import "testing"

func TestStableIDIsDeterministic(t *testing.T) {
	first := StableID(SourceGit, "abc123")
	second := StableID(SourceGit, "abc123")
	if first != second {
		t.Fatalf("expected equal ids, got %q and %q", first, second)
	}
}

func TestStableIDSeparatesPartBoundaries(t *testing.T) {
	joined := StableID(SourceFile, "ab", "c")
	split := StableID(SourceFile, "a", "bc")
	if joined == split {
		t.Fatalf("expected distinct ids for different part boundaries, both were %q", joined)
	}
}

func TestStableIDDependsOnSource(t *testing.T) {
	if StableID(SourceGit, "x") == StableID(SourceFile, "x") {
		t.Fatal("expected ids from different sources to differ")
	}
}

func TestHeadlineSkipsBlankLines(t *testing.T) {
	ev := Event{Content: "\n  \n  fix login bug  \nbody"}
	if got := ev.Headline(); got != "fix login bug" {
		t.Fatalf("expected %q, got %q", "fix login bug", got)
	}
}

func TestHeadlineOfEmptyContent(t *testing.T) {
	if got := (Event{}).Headline(); got != "" {
		t.Fatalf("expected empty headline, got %q", got)
	}
}

func TestRevision(t *testing.T) {
	if revision, ok := (Event{Metadata: Metadata{RevisionKey: "1727262000000"}}).Revision(); !ok || revision != 1727262000000 {
		t.Fatalf("expected the numeric revision, got %d %v", revision, ok)
	}
	if _, ok := (Event{Metadata: Metadata{RevisionKey: "x"}}).Revision(); ok {
		t.Fatal("expected a non-numeric revision to be absent")
	}
}
