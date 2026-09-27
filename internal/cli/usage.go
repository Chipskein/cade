package cli

// usageFor returns the help text in language.
func usageFor(language Language) string {
	return language.pick(usagePortuguese, usageEnglish)
}

const usagePortuguese = `cade — histórico pessoal local (git, browser, arquivos, teams)

Uso:
  cade [--config ARQUIVO] [--verbose] <comando> [argumentos]

Comandos:
  init                                  acha históricos, caches do Teams e repositórios,
                                        pergunta o que incluir e cria a configuração
  doctor                                confere modelos, FTS5, banco e caminhos, e diz
                                        o que corrigir (não altera o banco)
  ingest <fonte|all> [ALVO...]          ingere uma fonte (git, browser, file, teams);
                                        sem ALVO usa os alvos configurados
  timeline [--source F] [--all-authors] DATA [DATA_FIM]
                                        eventos de um dia ou intervalo (DATA: AAAA-MM-DD,
                                        hoje, ontem); commits de outros autores só com
                                        --all-authors
  ask [--source F] [--from D] [--to D] [--no-filters] [--json] PERGUNTA
                                        pergunta em linguagem natural (inclusive
                                        sobre tarefas: "quais tarefas finalizei?")
  tasks [--all] [DATA] [DATA_FIM]       tarefas trabalhadas (links de tarefa); PR aberto
                                        = concluída (padrão: hoje)
  reindex                               recalcula os vetores com o modelo de embedding
                                        configurado (após trocar de modelo); retomável
  forget <fonte>                        apaga os eventos de uma fonte, para reingerir;
                                        o que já saiu da fonte (ex.: cache do Teams
                                        expirado) não volta
  teams-schema DIR...                   estrutura (sem valores) de um IndexedDB
                                        do Chrome, p/ desenhar o ingestor do Teams
  help                                  mostra esta ajuda

Exemplos:
  cade ingest git ~/src/meu-projeto
  cade ingest browser ~/.mozilla/firefox/xyz.default/places.sqlite
  cade ingest all
  cade timeline ontem
  cade timeline 2026-09-01 2026-09-07
  cade ask --source browser --from 2026-09-19 "o que pesquisei sobre sqlite?"

Flags de cada comando: cade <comando> -h
`

const usageEnglish = `cade — local personal history (git, browser, files, teams)

Usage:
  cade [--config FILE] [--verbose] <command> [arguments]

Commands:
  init                                  finds histories, Teams caches and repositories,
                                        asks what to include and writes the config
  doctor                                checks models, FTS5, database and paths, and
                                        says what to fix (does not change the database)
  ingest <source|all> [TARGET...]       ingests a source (git, browser, file, teams);
                                        without TARGET uses the configured targets
  timeline [--source S] [--all-authors] DATE [END_DATE]
                                        events of a day or range (DATE: YYYY-MM-DD,
                                        hoje, ontem); other authors' commits only with
                                        --all-authors
  ask [--source S] [--from D] [--to D] [--no-filters] [--json] QUESTION
                                        natural-language question, in English or
                                        Portuguese (also about tasks: "which tasks did I finish?")
  tasks [--all] [DATE] [END_DATE]       tasks worked on (task links); PR opened
                                        = finished (default: today)
  reindex                               recomputes vectors with the configured
                                        embedding model (after changing it); resumable
  forget <source>                       deletes a source's events, to re-ingest;
                                        what left the source (e.g. an expired Teams
                                        cache) does not come back
  teams-schema DIR...                   structure (no values) of a Chrome IndexedDB,
                                        to design the Teams ingestor
  help                                  shows this help

Examples:
  cade ingest git ~/src/my-project
  cade ingest browser ~/.mozilla/firefox/xyz.default/places.sqlite
  cade ingest all
  cade timeline ontem
  cade timeline 2026-09-01 2026-09-07
  cade ask --source browser --from 2026-09-19 "what did I search about sqlite?"

Labels follow the locale, or ui.language in the config; answers follow
the question's language. Flags of each command: cade <command> -h
`
