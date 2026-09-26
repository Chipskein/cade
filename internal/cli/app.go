// Package cli implements the `cade` command line (RF5). All I/O-bearing
// dependencies arrive through Toolkit so commands are testable with fakes.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

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
	WriteConfig       func(path string) error
	OpenStore         func(ctx context.Context, path string) (storage.EventStore, error)
	LoadEmbedder      func(settings config.EmbeddingConfig) (ClosableEmbedder, error)
	LoadGenerator     func(settings config.ModelConfig) (ClosableGenerator, error)
	Sources           func(cfg config.Config) []ingest.SourceSpec
	ReadIndexedDB     func(dir string) ([]indexeddb.Record, error)
	// StderrIsTerminal selects in-place progress lines over periodic ones.
	StderrIsTerminal bool
	Now              func() time.Time
}

// commandEnv is what every subcommand receives after global flags are
// parsed.
type commandEnv struct {
	toolkit    Toolkit
	configPath string
	stdout     io.Writer
	stderr     io.Writer
	logger     *slog.Logger
}

type subcommand func(ctx context.Context, env commandEnv, args []string) error

var errUsage = errors.New("usage error")

// Run executes the command line in args (without the program name) and
// returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, toolkit Toolkit) int {
	env, rest, err := parseGlobalFlags(args, stdout, stderr, toolkit)
	if err != nil {
		return exitCode(err, stderr)
	}
	if len(rest) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	command, found := subcommands()[rest[0]]
	if !found {
		fmt.Fprintf(stderr, "comando desconhecido %q\n\n%s", rest[0], usageText)
		return 2
	}
	return exitCode(command(ctx, env, rest[1:]), stderr)
}

func subcommands() map[string]subcommand {
	return map[string]subcommand{
		"init":         runInit,
		"ingest":       runIngest,
		"timeline":     runTimeline,
		"ask":          runAsk,
		"teams-schema": runTeamsSchema,
		"forget":       runForget,
		"tasks":        runTasks,
	}
}

func parseGlobalFlags(args []string, stdout, stderr io.Writer, toolkit Toolkit) (commandEnv, []string, error) {
	flags := newFlagSet("cade", stderr)
	configPath := flags.String("config", "", "arquivo de configuração (padrão: ~/.config/cade/config.json)")
	verbose := flags.Bool("verbose", false, "logs de depuração em JSON no stderr")
	if err := flags.Parse(args); err != nil {
		return commandEnv{}, nil, errUsage
	}
	path, err := resolveConfigPath(*configPath, toolkit)
	if err != nil {
		return commandEnv{}, nil, err
	}
	env := commandEnv{toolkit: toolkit, configPath: path, stdout: stdout, stderr: stderr, logger: newLogger(stderr, *verbose)}
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

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func exitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errUsage) {
		return 2
	}
	// 130 is the conventional exit code for a Ctrl-C interruption.
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "interrompido")
		return 130
	}
	fmt.Fprintf(stderr, "erro: %v\n", err)
	return 1
}

func (env commandEnv) loadConfig() (config.Config, error) {
	return env.toolkit.LoadConfig(env.configPath)
}

func runInit(_ context.Context, env commandEnv, _ []string) error {
	if err := env.toolkit.WriteConfig(env.configPath); err != nil {
		return err
	}
	fmt.Fprintf(env.stdout, "Configuração criada em %s\nEdite as fontes (sources) e os caminhos dos modelos antes de ingerir.\n", env.configPath)
	return nil
}
