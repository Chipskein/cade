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
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/pacing"
	"github.com/chipskein/cade/internal/procctl"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
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
	// LoadImageDescriber loads the generation model with its vision
	// projector; only `ingest` with images on calls it.
	LoadImageDescriber func(generation config.ModelConfig, vision config.VisionConfig, logger *slog.Logger) (imagecaption.ClosableDescriber, error)
	// Sources builds the collectors; the file source reads captions, which
	// the first stage of `ingest` fills.
	Sources       func(cfg config.Config, captions ingest.ImageCaptions) []ingest.SourceSpec
	ReadIndexedDB func(dir string) ([]indexeddb.Record, error)
	// SchemaFiles stores the IndexedDB schemas `idb-discover` writes.
	SchemaFiles idbmap.SchemaFiles
	// StderrIsTerminal selects in-place progress lines over periodic ones.
	StderrIsTerminal bool
	// StdoutIsTerminal allows image previews, which are terminal art that
	// would only clutter a pipe or a file.
	StdoutIsTerminal bool
	// RenderImagePreview draws a cited image file as terminal text; "" when
	// it is not an image or no preview is possible (chafa missing).
	RenderImagePreview ImagePreviewer
	// Language of the help text and flag descriptions (from the locale).
	Language Language
	// DateOrder reads numeric dates in questions (from the locale).
	DateOrder timeline.DateOrder
	Now       func() time.Time
	// IngestRuns follows, stops and detaches `ingest` runs.
	IngestRuns IngestRunTools
}

// DetachedIngestVariable is set in the environment of the process that
// `ingest start` starts, which then runs as a background run.
const DetachedIngestVariable = "CADE_INGEST_DETACHED"

// IngestRunTools lets an `ingest` be followed, stopped and resumed from
// another terminal, and run in the background (issue #41).
type IngestRunTools struct {
	State ingestrun.StateFile
	Lock  ingestrun.Lock
	// LogPath receives the output of background runs.
	LogPath   string
	Processes procctl.Processes
	// Clock paces the models of background and gentle runs.
	Clock pacing.Clock
	// Detached is set in the process `ingest start` started.
	Detached bool
	PID      int
}

// commandEnv is what every subcommand receives after global flags are
// parsed.
type commandEnv struct {
	toolkit Toolkit
	// language is the locale's, or ui.language when the config sets it.
	language Language
	// dateOrder is the locale's, or ui.date_order when the config sets it.
	dateOrder  timeline.DateOrder
	configPath string
	stdout     io.Writer
	stderr     io.Writer
	logger     *slog.Logger
	// ingestRun is the `ingest` being recorded; nil for other commands.
	ingestRun *ingestRun
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
	env = withUISettings(env)
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

// withUISettings applies ui.language and ui.date_order over the locale's.
// A config that does not load keeps the locale: the command reports why.
func withUISettings(env commandEnv) commandEnv {
	env.language, env.dateOrder = env.toolkit.Language, env.toolkit.DateOrder
	cfg, err := env.toolkit.LoadConfig(env.configPath)
	if err != nil {
		return env
	}
	env.language = languageFromSetting(cfg.UI.Language, env.toolkit.Language)
	env.dateOrder = dateOrderFromSetting(cfg.UI.DateOrder, env.toolkit.DateOrder)
	return env
}

func subcommands() map[string]subcommand {
	return map[string]subcommand{
		"init":         runInit,
		"doctor":       runDoctor,
		"ingest":       runIngest,
		"timeline":     runTimeline,
		"ask":          runAsk,
		"teams-schema": runTeamsSchema,
		"idb-discover": runIDBDiscover,
		"idb-check":    runIDBCheck,
		"forget":       runForget,
		"tasks":        runTasks,
		"reindex":      runReindex,
		"compact":      runCompact,
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

// parseCommandFlags lets flags follow the arguments (`cade timeline ontem
// --source git`): the flag package stops at the first argument, so parsing
// resumes after each one. Everything after "--" is an argument. It returns
// the arguments in order.
func parseCommandFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, usageError(err)
		}
		rest := flags.Args()
		if len(rest) == 0 || endedFlags(args, rest) {
			return append(positional, rest...), nil
		}
		positional, args = append(positional, rest[0]), rest[1:]
	}
}

// endedFlags reports whether Parse stopped at a "--", which it consumes.
func endedFlags(args, rest []string) bool {
	consumed := len(args) - len(rest)
	return consumed > 0 && args[consumed-1] == "--"
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

// loadConfig reads the config, with the background limits during a
// background or gentle ingestion.
func (env commandEnv) loadConfig() (config.Config, error) {
	cfg, err := env.toolkit.LoadConfig(env.configPath)
	if err != nil || !env.ingestRun.isGentle() {
		return cfg, err
	}
	return cfg.WithBackgroundLimits(), nil
}
