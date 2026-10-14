package dbkey

import "context"

// The vaults with a file of their own keep it next to the database, so a
// config pointing at another database has its own key.
const (
	systemdCredsFileSuffix = ".key.cred"
	dpapiFileSuffix        = ".key.dpapi"
)

// platformVault builds the vault of one system, or reports there is none.
type platformVault func(ctx context.Context, databasePath string, runner CommandRunner) (KeyVault, bool)

// platformVaults are the secret stores by runtime.GOOS; FreeBSD and the
// rest have none, and the key is a passphrase there.
var platformVaults = map[string]platformVault{
	"linux": func(ctx context.Context, databasePath string, runner CommandRunner) (KeyVault, bool) {
		if !SystemdCredsUsable(ctx, runner) {
			return nil, false
		}
		return SystemdCreds{Runner: runner, Path: databasePath + systemdCredsFileSuffix}, true
	},
	"darwin": func(_ context.Context, _ string, runner CommandRunner) (KeyVault, bool) {
		return Keychain{Runner: runner}, true
	},
	"windows": func(_ context.Context, databasePath string, _ CommandRunner) (KeyVault, bool) {
		return DPAPIFile{Protector: WindowsDPAPI{}, Path: databasePath + dpapiFileSuffix}, true
	},
}

// PlatformVault is the secret store of goos for the database at path:
// systemd-creds on Linux with systemd 256 or newer, the Keychain on macOS,
// DPAPI on Windows. Without one (false), cade encrypt uses a passphrase.
//
//	vault, found := dbkey.PlatformVault(ctx, runtime.GOOS, path, dbkey.ExecRunner{})
func PlatformVault(ctx context.Context, goos, databasePath string, runner CommandRunner) (KeyVault, bool) {
	build, known := platformVaults[goos]
	if !known {
		return nil, false
	}
	return build(ctx, databasePath, runner)
}

// SearchOptions are the places NewSearch looks in.
type SearchOptions struct {
	Getenv func(string) string
	// Vault is the platform's, or nil where there is none.
	Vault    KeyVault
	Terminal PassphraseTerminal
	// Prompt asks for the typed passphrase, in the CLI's language.
	Prompt string
}

// NewSearch is cade's order: the variables first, so a recovery key or a
// passphrase overrides the vault (another machine, a timer without one);
// then the vault; last a passphrase typed at the terminal.
//
//	key, err := dbkey.NewSearch(dbkey.SearchOptions{Getenv: os.Getenv, Vault: vault, ...}).Load(ctx)
func NewSearch(options SearchOptions) Search {
	search := Search{RecoveryKeyFromEnv{Getenv: options.Getenv}, PassphraseFromEnv{Getenv: options.Getenv}}
	if options.Vault != nil {
		search = append(search, options.Vault)
	}
	return append(search, TypedPassphrase{Terminal: options.Terminal, Prompt: options.Prompt})
}
