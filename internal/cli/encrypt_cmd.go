package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/chipskein/cade/internal/dbcopy"
	"github.com/chipskein/cade/internal/dbkey"
	"github.com/chipskein/cade/internal/rootfs"
)

// keyChange is what `cade encrypt` and `cade decrypt` share: the database
// they rewrite and the ingestion lock they hold while doing it.
type keyChange struct {
	path    string
	release func()
}

// startKeyChange parses --yes, checks the database is in the state the
// command expects, asks to confirm and takes the ingestion lock, so no
// ingest writes to the file being replaced.
func (env commandEnv) startKeyChange(command string, args []string, wantEncrypted bool, question string) (keyChange, error) {
	yes, err := env.parseYes(command, args)
	if err != nil {
		return keyChange{}, err
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return keyChange{}, err
	}
	if err := env.requireEncryption(cfg.DatabasePath, wantEncrypted); err != nil {
		return keyChange{}, err
	}
	if confirmed, err := env.confirm(yes, question); err != nil || !confirmed {
		return keyChange{}, err
	}
	release, err := env.acquireIngestLock()
	return keyChange{path: cfg.DatabasePath, release: release}, err
}

// parseYes reads the only flag, --yes; the commands take no arguments.
func (env commandEnv) parseYes(command string, args []string) (bool, error) {
	flags := newFlagSet(command, env.stderr, env.language)
	yes := flags.Bool("yes", false, env.language.pick("confirma sem perguntar", "confirms without asking"))
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return false, err
	}
	if len(positional) != 0 {
		return false, fmt.Errorf(env.language.pick("cade %s não recebe argumentos, recebido %q", "cade %s takes no arguments, got %q"), command, positional)
	}
	return *yes, nil
}

func (env commandEnv) requireEncryption(path string, want bool) error {
	encrypted, err := env.toolkit.DatabaseEncrypted(path)
	switch {
	case err != nil:
		return err
	case encrypted && !want:
		return fmt.Errorf(env.language.pick("o banco %q já está cifrado", "database %q is already encrypted"), path)
	case !encrypted && want:
		return fmt.Errorf(env.language.pick("o banco %q não está cifrado", "database %q is not encrypted"), path)
	}
	return nil
}

// runEncrypt encrypts the database with a new key (#111): `cade encrypt`.
func runEncrypt(ctx context.Context, env commandEnv, args []string) error {
	change, err := env.startKeyChange("encrypt", args, false, env.language.pick(
		"Cifrar o banco? Sem a chave (ou a chave de recuperação) ele não abre mais. [s/N] ",
		"Encrypt the database? Without its key (or the recovery key) it no longer opens. [y/N] "))
	if err != nil || change.release == nil {
		return err
	}
	defer change.release()
	vault := env.toolkit.KeyVault(ctx, change.path)
	key, err := env.newDatabaseKey(ctx, vault)
	if err != nil {
		return err
	}
	backup, err := env.encryptDatabase(ctx, change.path, key)
	if err != nil {
		return err
	}
	env.reportEncrypted(change.path, backup, vault, key)
	return env.warnAboutPlainCopies(change.path)
}

// newDatabaseKey is a random key saved in vault and read back before the
// database changes, so a vault that loses it fails here; without a vault
// (nil), the passphrase is the key.
func (env commandEnv) newDatabaseKey(ctx context.Context, vault dbkey.KeyVault) ([]byte, error) {
	if vault == nil {
		return env.newPassphrase()
	}
	key, err := dbkey.NewKey(env.toolkit.Random)
	if err != nil {
		return nil, err
	}
	if err := vault.Save(ctx, key); err != nil {
		return nil, fmt.Errorf(env.language.pick("guardar a chave em %s: %w", "store the key in %s: %w"), vault.Location(), err)
	}
	if stored, err := vault.Load(ctx); err != nil || !bytes.Equal(stored, key) {
		return nil, fmt.Errorf(env.language.pick("a chave guardada em %s não foi lida de volta (%v); o banco não mudou",
			"the key stored in %s was not read back (%v); the database did not change"), vault.Location(), err)
	}
	return key, nil
}

func (env commandEnv) newPassphrase() ([]byte, error) {
	if passphrase := env.toolkit.Getenv(string(dbkey.PassphraseVariable)); passphrase != "" {
		return []byte(passphrase), nil
	}
	return dbkey.ChoosePassphrase(env.toolkit.PassphraseTerminal, dbkey.PassphrasePrompts{
		New:    env.language.pick("Nova frase-senha do banco: ", "New database passphrase: "),
		Repeat: env.language.pick("Repita a frase-senha: ", "Repeat the passphrase: "),
	})
}

// encryptDatabase rewrites the database under key and returns the copy it
// took before; without a database yet, it creates one encrypted, so no
// event is ever written in plain text.
func (env commandEnv) encryptDatabase(ctx context.Context, path string, key []byte) (string, error) {
	if _, err := fs.Stat(env.toolkit.RootFS, rootfs.Name(path)); errors.Is(err, fs.ErrNotExist) {
		store, err := env.toolkit.OpenStore(ctx, path, key, env.reportBackup)
		if err != nil {
			return "", err
		}
		return "", store.Close()
	}
	backup := dbcopy.BeforeEncrypt(path, env.toolkit.Now())
	return backup, env.toolkit.ReplaceDatabaseKey(ctx, path, nil, key, backup)
}

// reportEncrypted says where the key is and, with a vault, shows the
// recovery key: the only way back in when the vault loses it.
func (env commandEnv) reportEncrypted(path, backup string, vault dbkey.KeyVault, key []byte) {
	fmt.Fprintf(env.stdout, env.language.pick("Banco cifrado: %s\n", "Database encrypted: %s\n"), path)
	if backup != "" {
		fmt.Fprintf(env.stdout, env.language.pick("Cópia de antes, cifrada com a mesma chave: %s\n", "Copy from before, encrypted with the same key: %s\n"), backup)
	}
	if vault == nil {
		fmt.Fprintf(env.stdout, env.language.pick("A frase-senha é a chave: sem ela o banco não abre. Para o timer de ingestão, defina %s.\n",
			"The passphrase is the key: without it the database does not open. For the ingest timer, set %s.\n"), dbkey.PassphraseVariable)
		return
	}
	fmt.Fprintf(env.stdout, env.language.pick("Chave guardada em %s.\n", "Key stored in %s.\n"), vault.Location())
	fmt.Fprintf(env.stdout, env.language.pick(
		"\nChave de recuperação; guarde fora desta máquina (num gerenciador de senhas, no papel). Ela abre o banco onde o cofre não tem a chave, em %s:\n\n    %s\n\n",
		"\nRecovery key; keep it off this machine (in a password manager, on paper). It opens the database where the vault lacks the key, in %s:\n\n    %s\n\n"),
		dbkey.RecoveryKeyVariable, dbkey.FormatRecoveryKey(key))
}

// warnAboutPlainCopies lists the copies taken before the database was
// encrypted: they still hold it in plain text.
func (env commandEnv) warnAboutPlainCopies(path string) error {
	plain, err := env.copiesEncrypted(path, false)
	if err != nil || len(plain) == 0 {
		return err
	}
	fmt.Fprintln(env.stdout, env.language.pick("Estas cópias de antes continuam em texto puro:", "These earlier copies are still in plain text:"))
	writeCopies(env.stdout, plain)
	fmt.Fprintln(env.stdout, env.language.pick("Apague com `cade forget --backups`.", "Delete them with `cade forget --backups`."))
	return nil
}

// copiesEncrypted are the copies of the database at path that are (or,
// with encrypted false, are not) encrypted.
func (env commandEnv) copiesEncrypted(path string, encrypted bool) ([]dbcopy.Copy, error) {
	copies, err := dbcopy.Find(env.toolkit.RootFS, path)
	if err != nil {
		return nil, err
	}
	var matching []dbcopy.Copy
	for _, copied := range copies {
		state, err := env.toolkit.DatabaseEncrypted(copied.Path)
		if err != nil {
			return nil, err
		}
		if state == encrypted {
			matching = append(matching, copied)
		}
	}
	return matching, nil
}
