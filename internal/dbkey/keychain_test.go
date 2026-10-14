package dbkey

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestKeychainSavesAndLoadsTheKey(t *testing.T) {
	runner := &FakeSecurityRunner{}
	vault := Keychain{Runner: runner}
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	key, err := vault.Load(context.Background())
	if err != nil || !bytes.Equal(key, sampleKey) {
		t.Fatalf("key = %x, %v; want the saved one", key, err)
	}
	add := runner.Calls[0]
	if !strings.HasPrefix(add, "/usr/bin/security -i <add-generic-password -U -s cade-database -a cade -w ") {
		t.Errorf("save ran %q, want the interactive add with -U", add)
	}
}

func TestKeychainKeepsTheKeyOutOfTheArguments(t *testing.T) {
	runner := &FakeSecurityRunner{}
	vault := Keychain{Runner: runner}
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls {
		arguments, _, _ := strings.Cut(call, " <")
		if strings.Contains(arguments, hex.EncodeToString(sampleKey)) {
			t.Errorf("call %q has the key in its arguments", call)
		}
	}
}

func TestKeychainWithoutTheItemHoldsNoKey(t *testing.T) {
	_, err := Keychain{Runner: &FakeSecurityRunner{}}.Load(context.Background())
	if !errors.Is(err, ErrNoKey) || !strings.Contains(err.Error(), keychainService) {
		t.Errorf("err = %v, want ErrNoKey naming the item", err)
	}
}

func TestKeychainRejectsAnItemThatIsNotHex(t *testing.T) {
	runner := &FakeSecurityRunner{Item: "not hex!", Stored: true}
	_, err := Keychain{Runner: runner}.Load(context.Background())
	if err == nil || errors.Is(err, ErrNoKey) || !strings.Contains(err.Error(), "not hex") {
		t.Errorf("err = %v, want a malformed item error", err)
	}
}

func TestKeychainRemoveToleratesAMissingItem(t *testing.T) {
	runner := &FakeSecurityRunner{}
	vault := Keychain{Runner: runner}
	if err := vault.Save(context.Background(), sampleKey); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := vault.Remove(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if runner.Stored {
		t.Error("the item is still stored after remove")
	}
}

func TestKeychainPassesOtherFailuresOn(t *testing.T) {
	locked := Keychain{Runner: failingRunner{err: fakeExitError{code: 51}}}
	if _, err := locked.Load(context.Background()); err == nil || errors.Is(err, ErrNoKey) {
		t.Errorf("load err = %v, want the failure, not ErrNoKey", err)
	}
	if err := locked.Remove(context.Background()); err == nil {
		t.Error("remove succeeded on a failing security, want the failure")
	}
}

// failingRunner fails every call with err, as a locked Keychain would.
type failingRunner struct {
	err error
}

func (f failingRunner) Run(context.Context, []byte, string, ...string) ([]byte, error) {
	return nil, f.err
}
