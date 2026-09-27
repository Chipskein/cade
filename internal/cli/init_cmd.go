package cli

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/discovery"
	"github.com/chipskein/cade/internal/rootfs"
)

// runInit finds the browser histories, Teams caches and git repositories
// on this machine, asks which to include and writes the config: `cade
// init`. It reads names only, never content.
func runInit(_ context.Context, env commandEnv, args []string) error {
	language := env.language
	if len(args) != 0 {
		return fmt.Errorf(language.pick("cade init não recebe argumentos, recebido %q", "cade init takes no arguments, got %q"), args)
	}
	if _, err := fs.Stat(env.toolkit.RootFS, rootfs.Name(env.configPath)); err == nil {
		return fmt.Errorf(language.pick("a configuração %q já existe; edite-a ou rode `cade doctor`", "config %q already exists; edit it or run `cade doctor`"), env.configPath)
	}
	home, err := env.toolkit.HomeDir()
	if err != nil {
		return fmt.Errorf("locate home directory: %w", err)
	}
	cfg := config.Defaults()
	cfg.Sources = env.askSources(newInitPrompt(env.toolkit.Stdin, env.stdout, language), home, cfg.Sources)
	if err := env.toolkit.WriteConfig(env.configPath, cfg); err != nil {
		return err
	}
	env.printInitSummary(cfg.Sources)
	return nil
}

// askSources fills the source lists from the user's choices, written as
// "~/..." like the defaults.
func (env commandEnv) askSources(prompt *initPrompt, home string, sources config.SourcesConfig) config.SourcesConfig {
	fsys, language := env.toolkit.RootFS, env.language
	browsers := contractAll(discovery.BrowserHistories(fsys, home), home)
	sources.BrowserHistories = chooseFound(prompt, language.pick("Históricos de navegador", "Browser histories"), browsers, true)
	sources.TeamsIndexedDBDirs = chooseTeams(prompt, contractAll(discovery.TeamsCaches(fsys, home), home))
	sources.GitRepositories = env.askRepositories(prompt, home, sources.IgnoredDirNames)
	folders := prompt.lines(language.pick("\nPastas de notas ou documentos a indexar, uma por linha (linha vazia termina):",
		"\nNote or document folders to index, one per line (an empty line ends):"))
	sources.Directories = contractAll(resolveAll(folders, home), home)
	return sources
}

// chooseFound asks about candidates, or says none were found.
func chooseFound(prompt *initPrompt, title string, candidates []string, includeAll bool) []string {
	if len(candidates) == 0 {
		fmt.Fprintf(prompt.out, "\n%s: %s\n", title, prompt.language.pick("nenhum encontrado", "none found"))
		return []string{}
	}
	return prompt.choose(title, candidates, includeAll)
}

// chooseTeams defaults to none: the cache holds other people's messages,
// which the user should include knowingly.
func chooseTeams(prompt *initPrompt, caches []string) []string {
	language := prompt.language
	if len(caches) != 0 {
		fmt.Fprintf(prompt.out, "\n%s\n", language.pick(
			"O Teams guarda mensagens de outras pessoas: confira a política de dados da sua organização antes de incluí-lo.",
			"Teams keeps other people's messages: check your organization's data policy before including it."))
	}
	return chooseFound(prompt, language.pick("Caches do Teams (web, no Chrome)", "Teams caches (web, in Chrome)"), caches, false)
}

func (env commandEnv) askRepositories(prompt *initPrompt, home string, skipped []string) []string {
	language := prompt.language
	root := prompt.line(language.pick("\nDiretório onde procurar repositórios git (ex.: ~/src; vazio pula): ",
		"\nDirectory to search for git repositories (e.g. ~/src; empty skips): "))
	if root == "" {
		return []string{}
	}
	found := discovery.GitRepositories(env.toolkit.RootFS, resolvePath(root, home), skipped)
	title := fmt.Sprintf(language.pick("Repositórios git em %s", "Git repositories in %s"), root)
	return chooseFound(prompt, title, contractAll(found, home), true)
}

var (
	historyNoun    = nounForms{"histórico", "históricos", "history", "histories"}
	teamsCacheNoun = nounForms{"cache do Teams", "caches do Teams", "Teams cache", "Teams caches"}
	repositoryNoun = nounForms{"repositório", "repositórios", "repository", "repositories"}
	folderNoun     = nounForms{"pasta", "pastas", "folder", "folders"}
)

// initSourcesLine counts what init configured: "Fontes: 1 histórico, …".
func initSourcesLine(sources config.SourcesConfig, language Language) string {
	return fmt.Sprintf(language.pick("Fontes: %s, %s, %s, %s.\n", "Sources: %s, %s, %s, %s.\n"),
		language.count(len(sources.BrowserHistories), historyNoun), language.count(len(sources.TeamsIndexedDBDirs), teamsCacheNoun),
		language.count(len(sources.GitRepositories), repositoryNoun), language.count(len(sources.Directories), folderNoun))
}

func (env commandEnv) printInitSummary(sources config.SourcesConfig) {
	language := env.language
	fmt.Fprintf(env.stdout, language.pick("\nConfiguração criada em %s (só o seu usuário lê).\n", "\nConfig written to %s (readable by your user only).\n"), env.configPath)
	fmt.Fprint(env.stdout, initSourcesLine(sources, language))
	fmt.Fprint(env.stdout, language.pick(
		"Próximos passos:\n  cade doctor        confere modelos, banco e caminhos\n  cade ingest all    importa as fontes\n",
		"Next steps:\n  cade doctor        checks models, database and paths\n  cade ingest all    imports the sources\n"))
}

// resolvePath makes a typed path absolute: "~" is home, a relative path
// is under the working directory.
func resolvePath(path, home string) string {
	absolute, err := filepath.Abs(config.ExpandHomeIn(path, home))
	if err != nil {
		return path
	}
	return absolute
}

func resolveAll(paths []string, home string) []string {
	resolved := make([]string, len(paths))
	for i, path := range paths {
		resolved[i] = resolvePath(path, home)
	}
	return resolved
}

func contractAll(paths []string, home string) []string {
	contracted := make([]string, len(paths))
	for i, path := range paths {
		contracted[i] = config.ContractHome(path, home)
	}
	return contracted
}
