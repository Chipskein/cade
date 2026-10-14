package dbkey_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/dbkey"
	"github.com/chipskein/cade/internal/testfakes"
)

func TestNewKeyReadsKeySizeBytes(t *testing.T) {
	key, err := dbkey.NewKey(bytes.NewReader(bytes.Repeat([]byte{7}, 40)))
	if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{7}, dbkey.KeySize)) {
		t.Errorf("key = %x, %v; want %d bytes of 7", key, err, dbkey.KeySize)
	}
}

func TestNewKeyFailsOnAShortRandomSource(t *testing.T) {
	if _, err := dbkey.NewKey(bytes.NewReader([]byte{1, 2})); err == nil || !strings.Contains(err.Error(), "32-byte") {
		t.Errorf("err = %v, want it to name the key size", err)
	}
}

func TestSearchReturnsTheFirstKeyFound(t *testing.T) {
	empty := &testfakes.FakeKeyVault{Place: "empty"}
	holding := &testfakes.FakeKeyVault{Place: "holding", Key: []byte("secret")}
	later := &testfakes.FakeKeyVault{Place: "later", Key: []byte("other")}
	key, err := dbkey.Search{empty, holding, later}.Load(context.Background())
	if err != nil || string(key) != "secret" {
		t.Errorf("key = %q, %v; want the first one found", key, err)
	}
}

func TestSearchNamesEveryPlaceWhenNoneHasAKey(t *testing.T) {
	search := dbkey.Search{&testfakes.FakeKeyVault{Place: "the variable X"}, &testfakes.FakeKeyVault{Place: "systemd-creds"}}
	_, err := search.Load(context.Background())
	if !errors.Is(err, dbkey.ErrNoKey) || !strings.Contains(err.Error(), "looked in: the variable X; systemd-creds") {
		t.Errorf("err = %v, want ErrNoKey listing both places", err)
	}
}

func TestSearchStopsAtASourceThatFails(t *testing.T) {
	broken := &testfakes.FakeKeyVault{Place: "Keychain", Failure: errors.New("locked")}
	later := &testfakes.FakeKeyVault{Place: "later", Key: []byte("other")}
	_, err := dbkey.Search{broken, later}.Load(context.Background())
	if err == nil || errors.Is(err, dbkey.ErrNoKey) || !strings.Contains(err.Error(), "Keychain: locked") {
		t.Errorf("err = %v, want the failure with its place, not a fallback", err)
	}
}
