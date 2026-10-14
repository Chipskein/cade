package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/dbcopy"
)

// runDecrypt puts the database back in plain text and removes its key
// from the vault (#111): `cade decrypt`.
func runDecrypt(ctx context.Context, env commandEnv, args []string) error {
	change, err := env.startKeyChange("decrypt", args, true, env.language.pick(
		"Decifrar o banco? Ele e a cópia de antes ficam em texto puro. [s/N] ",
		"Decrypt the database? It and the copy from before will be in plain text. [y/N] "))
	if err != nil || change.release == nil {
		return err
	}
	defer change.release()
	key, source, err := env.databaseKey(ctx, change.path)
	if err != nil {
		return err
	}
	backup := dbcopy.BeforeDecrypt(change.path, env.toolkit.Now())
	if err := env.toolkit.ReplaceDatabaseKey(ctx, change.path, key, nil, backup); err != nil {
		return env.keyFrom(err, source)
	}
	fmt.Fprintf(env.stdout, env.language.pick("Banco em texto puro: %s\nCópia de antes, em texto puro: %s\n",
		"Database in plain text: %s\nCopy from before, in plain text: %s\n"), change.path, backup)
	if err := env.removeDatabaseKey(ctx, change.path); err != nil {
		return err
	}
	return env.warnAboutEncryptedCopies(change.path)
}

// removeDatabaseKey empties the vault only after the database no longer
// needs the key.
func (env commandEnv) removeDatabaseKey(ctx context.Context, path string) error {
	vault := env.toolkit.KeyVault(ctx, path)
	if vault == nil {
		return nil
	}
	if err := vault.Remove(ctx); err != nil {
		return fmt.Errorf(env.language.pick("remover a chave de %s: %w", "remove the key from %s: %w"), vault.Location(), err)
	}
	fmt.Fprintf(env.stdout, env.language.pick("Chave removida de %s.\n", "Key removed from %s.\n"), vault.Location())
	return nil
}

// warnAboutEncryptedCopies lists the copies still encrypted: without the
// key in the vault, only the recovery key opens them.
func (env commandEnv) warnAboutEncryptedCopies(path string) error {
	encrypted, err := env.copiesEncrypted(path, true)
	if err != nil || len(encrypted) == 0 {
		return err
	}
	fmt.Fprintln(env.stdout, env.language.pick("Estas cópias continuam cifradas e agora só abrem com a chave de recuperação:",
		"These copies stay encrypted and now open only with the recovery key:"))
	writeCopies(env.stdout, encrypted)
	fmt.Fprintln(env.stdout, env.language.pick("Apague com `cade forget --backups`.", "Delete them with `cade forget --backups`."))
	return nil
}
