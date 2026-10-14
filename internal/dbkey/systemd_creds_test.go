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

func newSystemdCreds(t *testing.T) (SystemdCreds, *FakeSystemdCredsRunner) {
	t.Helper()
	runner := &FakeSystemdCredsRunner{}
	return SystemdCreds{Runner: runner, Path: filepath.Join(t.TempDir(), "cade.db.key.cred")}, runner
}

func TestSystemdCredsSavesAndLoadsTheKey(t *testing.T) {
	vault, runner := newSystemdCreds(t)
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	key, err := vault.Load(context.Background())
	if err != nil || !bytes.Equal(key, sampleKey) {
		t.Fatalf("key = %x, %v; want the saved one", key, err)
	}
	for _, call := range runner.Calls {
		if !strings.Contains(call, "--user") || !strings.Contains(call, "--name=cade-db") {
			t.Errorf("call %q lacks --user or the credential name", call)
		}
	}
	info, err := os.Stat(vault.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != credentialFileMode {
		t.Errorf("credential mode = %o, want %o", info.Mode().Perm(), credentialFileMode)
	}
}

func TestSystemdCredsWithoutAFileHoldsNoKey(t *testing.T) {
	vault, runner := newSystemdCreds(t)
	_, err := vault.Load(context.Background())
	if !errors.Is(err, ErrNoKey) || !strings.Contains(err.Error(), vault.Path) || len(runner.Calls) != 0 {
		t.Errorf("err = %v, calls %q; want ErrNoKey naming the file, without running systemd-creds", err, runner.Calls)
	}
}

func TestSystemdCredsFailedSaveKeepsTheOldCredential(t *testing.T) {
	vault, runner := newSystemdCreds(t)
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	runner.FailOn = "encrypt"
	if err := vault.Save(context.Background(), bytes.Repeat([]byte{0x22}, KeySize)); err == nil {
		t.Fatal("save succeeded, want the encrypt failure")
	}
	runner.FailOn = ""
	if key, err := vault.Load(context.Background()); err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("key = %x, %v; want the first key still there", key, err)
	}
}

func TestSystemdCredsRemoveDeletesTheFileAndToleratesNone(t *testing.T) {
	vault, _ := newSystemdCreds(t)
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := vault.Remove(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := vault.Load(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Errorf("err = %v after remove, want ErrNoKey", err)
	}
}

func TestSystemdCredsUsableNeedsSystemd256(t *testing.T) {
	cases := map[string]bool{"systemd 262 (262-1-arch)": true, "systemd 256 (256.7)": true, "systemd 255 (255.4-1ubuntu8)": false, "zsh: not found": false}
	for version, want := range cases {
		if got := SystemdCredsUsable(context.Background(), &FakeSystemdCredsRunner{Version: version}); got != want {
			t.Errorf("usable with %q = %v, want %v", version, got, want)
		}
	}
	if SystemdCredsUsable(context.Background(), &FakeSystemdCredsRunner{FailOn: "--version"}) {
		t.Error("usable without a systemd-creds to run, want false")
	}
}

func TestSystemdCredsLocationNamesTheFile(t *testing.T) {
	vault, _ := newSystemdCreds(t)
	if !strings.Contains(vault.Location(), "systemd-creds --user") || !strings.Contains(vault.Location(), vault.Path) {
		t.Errorf("location = %q, want the tool and the file", vault.Location())
	}
}

// The fake stands in everywhere; this runs the real tool where it has
// --user (the dev machine; CI's Ubuntu has systemd 255).
func TestSystemdCredsWithTheRealTool(t *testing.T) {
	if !SystemdCredsUsable(context.Background(), ExecRunner{}) {
		t.Skip("no systemd-creds with --user here")
	}
	vault := SystemdCreds{Runner: ExecRunner{}, Path: filepath.Join(t.TempDir(), "cade.db.key.cred")}
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(vault.Path)
	if err != nil || bytes.Contains(sealed, sampleKey) {
		t.Fatalf("credential file holds the raw key (read err %v)", err)
	}
	if key, err := vault.Load(context.Background()); err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("key = %x, %v; want the saved one", key, err)
	}
}
