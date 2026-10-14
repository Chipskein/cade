// Package dbkey keeps the key that encrypts cade's database (#111) outside
// the database: in the system's secret store (systemd-creds, Keychain,
// DPAPI) or, where there is none, in a passphrase the user supplies.
package dbkey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// KeySize is the length of a generated key: 256 bits, which sqlite3mc's
// key derivation takes as is.
const KeySize = 32

// ErrNoKey is wrapped when a source holds no key.
var ErrNoKey = errors.New("no database key")

// KeySource is a place a key can be read from.
type KeySource interface {
	// Load returns the key, or an error wrapping ErrNoKey when the source
	// holds none.
	Load(ctx context.Context) ([]byte, error)
	// Location says where the key is looked for, for messages, e.g.
	// "systemd-creds --user, in /home/me/.local/share/cade/cade.db.key.cred".
	Location() string
}

// KeyVault is a KeySource that can also store the key: what `cade encrypt`
// writes to and `cade decrypt` empties.
type KeyVault interface {
	KeySource
	// Save stores key, replacing the one there.
	Save(ctx context.Context, key []byte) error
	// Remove deletes the stored key; nothing stored is not an error.
	Remove(ctx context.Context) error
}

// NewKey reads a random key from random (crypto/rand.Reader outside tests).
//
//	key, err := dbkey.NewKey(rand.Reader)
func NewKey(random io.Reader) ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, fmt.Errorf("generate a %d-byte database key: %w", KeySize, err)
	}
	return key, nil
}

// Search is the sources a key is looked for in, in order.
type Search []KeySource

// Load returns the key of the first source that has one. When none has,
// the error lists every place looked in.
//
//	key, err := dbkey.Search{recoveryFromEnv, vault}.Load(ctx)
func (s Search) Load(ctx context.Context) ([]byte, error) {
	for _, source := range s {
		key, err := source.Load(ctx)
		if err == nil {
			return key, nil
		}
		if !errors.Is(err, ErrNoKey) {
			return nil, fmt.Errorf("read the database key from %s: %w", source.Location(), err)
		}
	}
	return nil, fmt.Errorf("%w; looked in: %s", ErrNoKey, s.Locations())
}

// Locations lists where s looks, in order, for messages.
func (s Search) Locations() string {
	locations := make([]string, 0, len(s))
	for _, source := range s {
		locations = append(locations, source.Location())
	}
	return strings.Join(locations, "; ")
}
