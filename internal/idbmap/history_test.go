package idbmap

import (
	"errors"
	"testing"
)

func savedDir(t *testing.T) (SchemaDir, *FakeSchemaFiles) {
	t.Helper()
	files := NewFakeSchemaFiles()
	dir := NewSchemaDir(files, schemaDirPath)
	if err := dir.Save(savedSchema(t), false); err != nil {
		t.Fatal(err)
	}
	return dir, files
}

func regenerated(t *testing.T, sender Path) Schema {
	t.Helper()
	next := savedSchema(t)
	rule := next.Fields[FieldSender]
	rule.Paths = []Path{sender}
	next.Fields[FieldSender] = rule
	return next
}

func senderPath(t *testing.T, dir SchemaDir) Path {
	t.Helper()
	current, err := dir.Load("whatsapp")
	if err != nil {
		t.Fatal(err)
	}
	return current.Fields[FieldSender].Paths[0]
}

func TestReplaceKeepsTheCurrentSchemaInHistory(t *testing.T) {
	dir, files := savedDir(t)
	saved, err := dir.Replace(regenerated(t, "$.author.user"))
	if err != nil || saved.Revision != 2 || senderPath(t, dir) != "$.author.user" {
		t.Fatalf("expected revision 2 saved, got %+v (err %v)", saved, err)
	}
	if _, kept := files.Files[dir.HistoryPath("whatsapp", 1)]; !kept {
		t.Fatal("expected revision 1 kept in history")
	}
}

func TestRollbackRestoresTheEarlierRevision(t *testing.T) {
	dir, _ := savedDir(t)
	_, _ = dir.Replace(regenerated(t, "$.author.user"))
	restored, err := dir.Rollback("whatsapp")
	if err != nil || restored.Revision != 1 || senderPath(t, dir) != "$.author._serialized" {
		t.Fatalf("expected revision 1 back, got %+v (err %v)", restored, err)
	}
	if _, err := dir.Rollback("whatsapp"); !errors.Is(err, ErrNoEarlierRevision) {
		t.Fatalf("expected no earlier revision, got %v", err)
	}
}

func TestReplaceAfterRollbackNumbersAboveEveryRevision(t *testing.T) {
	dir, _ := savedDir(t)
	_, _ = dir.Replace(regenerated(t, "$.author.user"))
	_, _ = dir.Rollback("whatsapp")
	saved, err := dir.Replace(regenerated(t, "$.from"))
	if err != nil || saved.Revision != 3 {
		t.Fatalf("expected revision 3 after 1 and 2, got %+v (err %v)", saved, err)
	}
}

func TestSaveCandidateIsNotASchemaSource(t *testing.T) {
	dir, files := savedDir(t)
	path, err := dir.SaveCandidate(regenerated(t, "$.from"))
	if err != nil || path != dir.CandidatePath("whatsapp") || files.Files[path] == nil {
		t.Fatalf("expected the candidate written, got %q (err %v)", path, err)
	}
	if names, _ := dir.Names(); len(names) != 1 || senderPath(t, dir) != "$.author._serialized" {
		t.Fatalf("expected the current schema untouched, got %v", names)
	}
}

func TestReplaceRequiresASavedSchema(t *testing.T) {
	if _, err := NewSchemaDir(NewFakeSchemaFiles(), schemaDirPath).Replace(savedSchema(t)); err == nil {
		t.Fatal("expected an error replacing a schema never saved")
	}
}
