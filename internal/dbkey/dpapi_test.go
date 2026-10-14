package dbkey

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newDPAPIFile(t *testing.T) DPAPIFile {
	t.Helper()
	return DPAPIFile{Protector: FakeBlobProtector{}, Path: filepath.Join(t.TempDir(), "cade.db.key.dpapi")}
}

func TestDPAPIFileSavesTheKeySealed(t *testing.T) {
	vault := newDPAPIFile(t)
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(vault.Path)
	if err != nil || !bytes.HasPrefix(sealed, []byte(sealedPrefix)) {
		t.Fatalf("file = %q, %v; want the sealed key", sealed, err)
	}
	if info, _ := os.Stat(vault.Path); runtime.GOOS != "windows" && info.Mode().Perm() != keyFileMode {
		t.Errorf("key file mode = %o, want %o", info.Mode().Perm(), keyFileMode)
	}
	if key, err := vault.Load(context.Background()); err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("key = %x, %v; want the saved one", key, err)
	}
}

func TestDPAPIFileRefusesAFileItDidNotSeal(t *testing.T) {
	vault := newDPAPIFile(t)
	if err := os.WriteFile(vault.Path, sampleKey, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Load(context.Background()); err == nil || errors.Is(err, ErrNoKey) {
		t.Errorf("err = %v, want the unseal failure", err)
	}
}

func TestDPAPIFileWithoutAFileHoldsNoKey(t *testing.T) {
	vault := newDPAPIFile(t)
	if _, err := vault.Load(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Errorf("err = %v, want ErrNoKey", err)
	}
	if err := vault.Remove(context.Background()); err != nil {
		t.Errorf("remove without a file: %v", err)
	}
	if !strings.Contains(vault.Location(), "DPAPI") || !strings.Contains(vault.Location(), vault.Path) {
		t.Errorf("location = %q, want DPAPI and the file", vault.Location())
	}
}

func TestWindowsDPAPIWorksOnlyOnWindows(t *testing.T) {
	blob, err := WindowsDPAPI{}.Protect(sampleKey)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("protect succeeded off Windows, want an error")
		}
		return
	}
	if err != nil || bytes.Contains(blob, sampleKey) {
		t.Fatalf("protect = %x, %v; want a sealed blob", blob, err)
	}
	if key, err := (WindowsDPAPI{}).Unprotect(blob); err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("unprotect = %x, %v; want the key back", key, err)
	}
}
