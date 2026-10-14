package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/dbkey"
	"github.com/chipskein/cade/internal/storage"
)

// databaseKey finds the key of the database at path when it is encrypted,
// and nil when it is not. The error lists every place looked in, and the
// source tells where a key the database then rejects came from (#111).
func (env commandEnv) databaseKey(ctx context.Context, path string) ([]byte, dbkey.KeySource, error) {
	encrypted, err := env.toolkit.DatabaseEncrypted(path)
	if err != nil || !encrypted {
		return nil, nil, err
	}
	prompt := env.language.pick("Frase-senha do banco: ", "Database passphrase: ")
	key, source, err := env.toolkit.DatabaseKeys(ctx, path, prompt).Locate(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf(env.language.pick("o banco %q está cifrado e a chave não foi encontrada: %w", "database %q is encrypted and its key was not found: %w"), path, err)
	}
	return key, source, nil
}

// keyFrom adds to err where the key the database rejected was read.
func (env commandEnv) keyFrom(err error, source dbkey.KeySource) error {
	if err == nil || source == nil {
		return err
	}
	return fmt.Errorf(env.language.pick("%w (chave lida de %s)", "%w (key read from %s)"), err, source.Location())
}

// openStore opens the database at path with its key, if it has one.
func (env commandEnv) openStore(ctx context.Context, path string) (storage.EventStore, error) {
	key, source, err := env.databaseKey(ctx, path)
	if err != nil {
		return nil, err
	}
	store, err := env.toolkit.OpenStore(ctx, path, key, env.reportBackup)
	return store, env.keyFrom(err, source)
}

// inspectDatabase is openStore for doctor, which reads without migrating.
func (env commandEnv) inspectDatabase(ctx context.Context, path string) (storage.DatabaseState, error) {
	key, source, err := env.databaseKey(ctx, path)
	if err != nil {
		return storage.DatabaseState{}, err
	}
	state, err := env.toolkit.InspectDatabase(ctx, path, key)
	return state, env.keyFrom(err, source)
}
