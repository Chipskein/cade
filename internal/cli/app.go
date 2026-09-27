// Package cli implements the `cade` command line (RF5). All I/O-bearing
// dependencies arrive through Toolkit so commands are testable with fakes.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"time"

	"github.com/chipskein/cade/internal/buildinfo"
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
)

// ClosableEmbedder is an embedder holding a model that must be released.
type ClosableEmbedder interface {
	llm.Embedder
	Close() error
}

// ClosableGenerator is a generator holding a model that must be released.
// It also produces grammar-constrained output, used to interpret questions.
type ClosableGenerator interface {
	llm.Generator
	llm.StructuredGenerator
	Close() error
}

// Toolkit supplies every external dependency a command may need. Models
// are loaded lazily, so `timeline` never pays for them.
type Toolkit struct {
	DefaultConfigPath func() (string, error)
	LoadConfig        func(path string) (config.Config, error)
	WriteConfig       func(path string, cfg config.Config) error
	// OpenStore migrates the database if needed; backupCreated hears where
	// a migration saved the copy.
	OpenStore func(ctx context.Context, path string, backupCreated func(backupPath string)) (storage.EventStore, error)
	// InspectDatabase reads the database without migrating it (doctor).
	InspectDatabase func(ctx context.Context, path string) (storage.DatabaseState, error)
	// RootFS is the filesystem at "/" (see rootfs), where init looks for
	// sources and doctor checks the configured paths.
	RootFS  fs.FS
	HomeDir func() (string, error)
	// Stdin answers init's questions.
	Stdin io.Reader
	// Build is what this binary was built from (`cade version`).
	Build         buildinfo.Info
	LoadEmbedder  func(settings config.EmbeddingConfig, logger *slog.Logger) (ClosableEmbedder, error)
	LoadGenerator func(settings config.ModelConfig, logger *slog.Logger) (ClosableGenerator, error)
	Sources       func(cfg config.Config) []ingest.SourceSpec
	ReadIndexedDB func(dir string) ([]indexeddb.Record, error)
	// StderrIsTerminal selects in-place progress lines over periodic ones.
	StderrIsTerminal bool
	// Language of the help text and flag descriptions (from the locale).
	Language Language
	Now      func() time.Time
}

// commandEnv is what every subcommand receives after global flags are
// parsed.
type commandEnv struct {
	toolkit Toolkit
	// language is the locale's, or ui.language when the config sets it.
	language   Language
	configPath string
	stdout     io.Writer
	stderr     io.Writer
	logger     *slog.Logger
}

type subcommand func(ctx context.Context, env commandEnv, args []string) error

var errUsage = errors.New("usage error")

// errHelpShown means -h printed the help: not a failure.
var errHelpShown = errors.New("help shown")

// usageError maps a flag parsing error: -h is a request, anything else a
// usage mistake (the flag package has already printed why).
func usageError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return errHelpShown
	}
	return errUsage
}

// Run executes the command line in args (without the program name) and
// returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, toolkit Toolkit) int {
	env, rest, err := parseGlobalFlags(args, stdout, stderr, toolkit)
	if err != nil {
		return exitCode(err, stderr, toolkit.Language)
	}
	env.language = configuredLanguage(env, toolkit.Language)
	usage := usageFor(env.language)
	if len(rest) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if rest[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	command, found := subcommands()[rest[0]]
	if !found {
		fmt.Fprintf(stderr, "%s %q\n\n%s", env.language.pick("comando desconhecido", "unknown command"), rest[0], usage)
		return 2
	}
	return exitCode(command(ctx, env, rest[1:]), stderr, env.language)
}

// configuredLanguage applies ui.language over the locale's language. A
// config that does not load keeps the locale: the command reports why.
func configuredLanguage(env commandEnv, fromLocale Language) Language {
	cfg, err := env.toolkit.LoadConfig(env.configPath)
	if err != nil {
		return fromLocale
	}
	return languageFromSetting(cfg.UI.Language, fromLocale)
}

func subcommands() map[string]subcommand {
	return map[string]subcommand{
		"init":         runInit,
		"doctor":       runDoctor,
		"ingest":       runIngest,
		"timeline":     runTimeline,
		"ask":          runAsk,
		"teams-schema": runTeamsSchema,
		"forget":       runForget,
		"tasks":        runTasks,
		"reindex":      runReindex,
		"version":      runVersion,
	}
}

func parseGlobalFlags(args []string, stdout, stderr io.Writer, toolkit Toolkit) (commandEnv, []string, error) {
	language := toolkit.Language
	flags := newFlagSet("cade", stderr, language)
	flags.Usage = func() { fmt.Fprint(stdout, usageFor(language)) }
	configPath := flags.String("config", "", language.pick("arquivo de configuração (padrão: ~/.config/cade/config.json)", "configuration file (default: ~/.config/cade/config.json)"))
	verbose := flags.Bool("verbose", false, language.pick("logs de depuração em JSON no stderr", "JSON debug logs on stderr"))
	showVersion := flags.Bool("version", false, language.pick("mostra a versão (como `cade version`)", "shows the version (like `cade version`)"))
	if err := flags.Parse(args); err != nil {
		return commandEnv{}, nil, usageError(err)
	}
	path, err := resolveConfigPath(*configPath, toolkit)
	if err != nil {
		return commandEnv{}, nil, err
	}
	env := commandEnv{toolkit: toolkit, language: language, configPath: path, stdout: stdout, stderr: stderr, logger: newLogger(stderr, *verbose)}
	if *showVersion {
		return env, []string{"version"}, nil
	}
	return env, flags.Args(), nil
}

func resolveConfigPath(explicit string, toolkit Toolkit) (string, error) {
	if explicit != "" {
		return config.ExpandHome(explicit), nil
	}
	return toolkit.DefaultConfigPath()
}

func newLogger(stderr io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level}))
}

// newFlagSet reports errors and -h on stderr, with a header in language.
func newFlagSet(name string, stderr io.Writer, language Language) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintf(stderr, language.pick("Uso de cade %s:\n", "Usage of cade %s:\n"), name)
		flags.PrintDefaults()
	}
	return flags
}

func exitCode(err error, stderr io.Writer, language Language) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errHelpShown) {
		return 0
	}
	if errors.Is(err, errUsage) {
		return 2
	}
	// 130 is the conventional exit code for a Ctrl-C interruption.
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, language.pick("interrompido", "interrupted"))
		return 130
	}
	fmt.Fprintf(stderr, "%s: %v\n", language.pick("erro", "error"), err)
	return 1
}

func (env commandEnv) loadConfig() (config.Config, error) {
	return env.toolkit.LoadConfig(env.configPath)
}
