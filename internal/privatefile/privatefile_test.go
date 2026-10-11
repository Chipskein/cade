package privatefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRestrictMakesAFileOwnerOnlyWhereModesExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restrict(path, 0o600); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || (ModesEnforced && info.Mode().Perm() != 0o600) {
		t.Fatalf("expected mode 600 where enforced, got %v (err %v)", info.Mode(), err)
	}
}

func TestRestrictReportsAMissingFile(t *testing.T) {
	if err := Restrict(filepath.Join(t.TempDir(), "absent"), 0o600); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}
