package dbkey

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeEnvironment is a fixed set of variables, read like os.Getenv.
type fakeEnvironment map[KeyVariable]string

func (f fakeEnvironment) Getenv(name string) string {
	return f[KeyVariable(name)]
}

func TestRecoveryKeyFromEnvParsesTheVariable(t *testing.T) {
	env := fakeEnvironment{RecoveryKeyVariable: FormatRecoveryKey(sampleKey)}
	key, err := RecoveryKeyFromEnv{Getenv: env.Getenv}.Load(context.Background())
	if err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("key = %x, %v; want the recovery key's", key, err)
	}
}

func TestRecoveryKeyFromEnvNamesTheVariableOfABadKey(t *testing.T) {
	env := fakeEnvironment{RecoveryKeyVariable: "AAAA"}
	_, err := RecoveryKeyFromEnv{Getenv: env.Getenv}.Load(context.Background())
	if !errors.Is(err, ErrBadRecoveryKey) || !strings.Contains(err.Error(), string(RecoveryKeyVariable)) {
		t.Errorf("err = %v, want ErrBadRecoveryKey naming %s", err, RecoveryKeyVariable)
	}
}

func TestPassphraseFromEnvIsTheKey(t *testing.T) {
	env := fakeEnvironment{PassphraseVariable: "correct horse"}
	key, err := PassphraseFromEnv{Getenv: env.Getenv}.Load(context.Background())
	if err != nil || string(key) != "correct horse" {
		t.Errorf("key = %q, %v; want the passphrase", key, err)
	}
}

func TestUnsetVariablesHoldNoKey(t *testing.T) {
	env := fakeEnvironment{}
	for _, source := range []KeySource{RecoveryKeyFromEnv{Getenv: env.Getenv}, PassphraseFromEnv{Getenv: env.Getenv}} {
		if _, err := source.Load(context.Background()); !errors.Is(err, ErrNoKey) {
			t.Errorf("%s: err = %v, want ErrNoKey", source.Location(), err)
		}
		if !strings.Contains(source.Location(), "CADE_DB_") {
			t.Errorf("location %q does not name the variable", source.Location())
		}
	}
}
