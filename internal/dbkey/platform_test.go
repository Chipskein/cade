package dbkey

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestPlatformVaultPerSystem(t *testing.T) {
	recent := &FakeSystemdCredsRunner{Version: "systemd 262 (262-1-arch)"}
	cases := []struct {
		goos   string
		runner CommandRunner
		want   KeyVault
	}{
		{"linux", recent, SystemdCreds{Runner: recent, Path: "/d/cade.db.key.cred"}},
		{"darwin", recent, Keychain{Runner: recent}},
		{"windows", recent, DPAPIFile{Protector: WindowsDPAPI{}, Path: "/d/cade.db.key.dpapi"}},
	}
	for _, c := range cases {
		vault, found := PlatformVault(context.Background(), c.goos, "/d/cade.db", c.runner)
		if !found || !reflect.DeepEqual(vault, c.want) {
			t.Errorf("%s: vault = %#v, %v; want %#v", c.goos, vault, found, c.want)
		}
	}
}

func TestPlatformVaultIsAbsentWithoutAStore(t *testing.T) {
	old := &FakeSystemdCredsRunner{Version: "systemd 255 (255.4-1ubuntu8)"}
	for _, goos := range []string{"freebsd", "linux"} {
		if vault, found := PlatformVault(context.Background(), goos, "/d/cade.db", old); found || vault != nil {
			t.Errorf("%s: vault = %#v, %v; want none (a passphrase)", goos, vault, found)
		}
	}
}

func TestNewSearchLooksInTheVariablesTheVaultAndTheTerminal(t *testing.T) {
	vault := Keychain{Runner: &FakeSecurityRunner{}}
	search := NewSearch(SearchOptions{Getenv: fakeEnvironment{}.Getenv, Vault: vault, Prompt: "Passphrase: "})
	want := "the variable CADE_DB_RECOVERY_KEY (a recovery key); the variable CADE_DB_PASSPHRASE (a passphrase); the login Keychain, item cade-database; a passphrase typed at the terminal"
	if got := search.Locations(); got != want {
		t.Errorf("locations = %q\nwant %q", got, want)
	}
	if without := NewSearch(SearchOptions{Getenv: fakeEnvironment{}.Getenv}); strings.Contains(without.Locations(), "Keychain") || len(without) != 3 {
		t.Errorf("without a vault: %q, want the variables and the terminal only", without.Locations())
	}
}

func TestNewSearchPrefersTheVariablesOverTheVault(t *testing.T) {
	runner := &FakeSecurityRunner{}
	vault := Keychain{Runner: runner}
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	env := fakeEnvironment{PassphraseVariable: "from the variable"}
	key, source, err := NewSearch(SearchOptions{Getenv: env.Getenv, Vault: vault}).Locate(context.Background())
	if err != nil || string(key) != "from the variable" || !strings.Contains(source.Location(), string(PassphraseVariable)) {
		t.Errorf("key = %q from %v, %v; want the variable's", key, source, err)
	}
}
