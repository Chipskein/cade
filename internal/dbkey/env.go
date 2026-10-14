package dbkey

import (
	"context"
	"fmt"
)

// KeyVariable names an environment variable a key is read from.
type KeyVariable string

const (
	// RecoveryKeyVariable holds a recovery key: it opens the database on a
	// machine whose secret store does not have the key.
	RecoveryKeyVariable KeyVariable = "CADE_DB_RECOVERY_KEY"
	// PassphraseVariable holds the passphrase where there is no secret
	// store (FreeBSD, Linux without systemd 256), e.g. for the timer.
	PassphraseVariable KeyVariable = "CADE_DB_PASSPHRASE"
)

// RecoveryKeyFromEnv reads a recovery key from RecoveryKeyVariable.
type RecoveryKeyFromEnv struct {
	Getenv func(string) string
}

// Load parses the variable; unset or empty is ErrNoKey.
func (r RecoveryKeyFromEnv) Load(context.Context) ([]byte, error) {
	text := r.Getenv(string(RecoveryKeyVariable))
	if text == "" {
		return nil, fmt.Errorf("%s is not set: %w", RecoveryKeyVariable, ErrNoKey)
	}
	key, err := ParseRecoveryKey(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", RecoveryKeyVariable, err)
	}
	return key, nil
}

func (r RecoveryKeyFromEnv) Location() string {
	return "the variable " + string(RecoveryKeyVariable) + " (a recovery key)"
}

// PassphraseFromEnv reads the passphrase from PassphraseVariable. The
// passphrase is the key: sqlite3mc derives the cipher key from it.
type PassphraseFromEnv struct {
	Getenv func(string) string
}

// Load returns the passphrase's bytes; unset or empty is ErrNoKey.
func (p PassphraseFromEnv) Load(context.Context) ([]byte, error) {
	passphrase := p.Getenv(string(PassphraseVariable))
	if passphrase == "" {
		return nil, fmt.Errorf("%s is not set: %w", PassphraseVariable, ErrNoKey)
	}
	return []byte(passphrase), nil
}

func (p PassphraseFromEnv) Location() string {
	return "the variable " + string(PassphraseVariable) + " (a passphrase)"
}
