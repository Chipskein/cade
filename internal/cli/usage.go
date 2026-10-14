package cli

// usageFor returns the help text in language.
func usageFor(language Language) string {
	return language.pick(usagePortuguese, usageEnglish)
}

const usagePortuguese = `cade — histórico pessoal local (git, browser, arquivos, teams)

Uso:
  cade [--config ARQUIVO] [--verbose] [--version] <comando> [argumentos]

Comandos:
  init                                  acha históricos, caches do Teams e repositórios,
                                        pergunta o que incluir e cria a configuração
  doctor                                confere modelos, FTS5, banco e caminhos, e diz
                                        o que corrigir (não altera o banco)
  ingest [--gentle] <fonte|all> [ALVO...]
                                        ingere uma fonte (git, browser, file, teams,
                                        whatsapp-backup ou um schema salvo);
                                        sem ALVO usa os alvos configurados; --gentle
                                        usa os limites de CPU/GPU de ingest.background
  ingest start <fonte|all> [ALVO...]    o mesmo em segundo plano, com esses limites;
                                        continua depois de fechar o terminal
  ingest status|pause|resume|stop       acompanha, pausa (modelos na memória),
                                        continua ou para a ingestão
  timeline [--source F] [--all-authors] DATA [DATA_FIM]
                                        eventos de um dia ou intervalo (DATA: AAAA-MM-DD,
                                        hoje, ontem); commits de outros autores só com
                                        --all-authors
  timeline --around UID [--window 2h]   o evento e o que aconteceu até 2h antes e depois
  ask [--source F] [--from D] [--to D] [--no-filters] [--json] PERGUNTA
                                        pergunta em linguagem natural (inclusive
                                        sobre tarefas: "quais tarefas finalizei?")
  tasks [--all] [DATA] [DATA_FIM]       tarefas trabalhadas (links de tarefa); PR aberto
                                        significa visita à criação, sem saber aprovação/merge
  reindex                               recalcula os vetores com o modelo de embedding
                                        configurado (após trocar de modelo); retomável
  reindex --captions                    descreve de novo as imagens cuja descrição veio
                                        de outro modelo ou prompt; retomável
  compact                               devolve ao disco o espaço dos vetores apagados
                                        (sem recalcular embeddings); precisa de espaço
                                        livre para uma cópia dos vetores
  encrypt [--yes]                       cifra o banco com uma chave guardada no cofre
                                        do sistema (ou uma frase-senha) e mostra a
                                        chave de recuperação
  decrypt [--yes]                       volta o banco para texto puro e apaga a chave
  forget <fonte> | --uid UID            apaga eventos de uma fonte ou um evento;
  forget --match TEXTO [--source F]     localiza eventos; --yes confirma em scripts
                                        (também aceita --from D --to D);
                                        o que já saiu da fonte (ex.: cache do Teams
                                        expirado) não volta
  forget --person NOME                  apaga tudo sobre uma pessoa (o que ela mandou,
                                        menções e chats só com ela), depois de listar
  forget --backups                      apaga as cópias do banco (antes de migrações),
                                        que ainda guardam o que foi esquecido
  export [--source F] [--from D --to D] tudo o que o cade guarda, em JSON Lines
                                        (um evento por linha, com os derivados);
                                        --person NOME: só o que é sobre ela
  teams-schema DIR...                   estrutura (sem valores) de um IndexedDB
                                        do Chrome, p/ desenhar o ingestor do Teams
  schema-discover --name N [--force] [--print] DIR
                                        gera com o modelo local um schema para
                                        ingerir o armazenamento de um aplicativo
                                        no navegador (IndexedDB); --rollback
                                        --name N volta à revisão anterior;
                                        antes idb-discover, que continua valendo
  schema-check [--update [--rekey]] [N...]
                                        confere se os aplicativos ainda batem com
                                        os schemas; --update regenera o que mudou;
                                        --rekey aceita um schema que muda os UIDs e
                                        renomeia os eventos já indexados (com cópia);
                                        antes idb-check, que continua valendo
  version                               versão, commit, data e tipo de build (CPU/CUDA)
  help                                  mostra esta ajuda

Exemplos:
  cade ingest git ~/src/meu-projeto
  cade ingest browser ~/.mozilla/firefox/xyz.default/places.sqlite
  cade ingest all
  cade ingest start file
  cade timeline ontem
  cade timeline 2026-09-01 2026-09-07
  cade ask --source browser --from 2026-09-19 "o que pesquisei sobre sqlite?"

Flags de cada comando (antes ou depois dos argumentos): cade <comando> -h
`

const usageEnglish = `cade — local personal history (git, browser, files, teams)

Usage:
  cade [--config FILE] [--verbose] [--version] <command> [arguments]

Commands:
  init                                  finds histories, Teams caches and repositories,
                                        asks what to include and writes the config
  doctor                                checks models, FTS5, database and paths, and
                                        says what to fix (does not change the database)
  ingest [--gentle] <source|all> [TARGET...]
                                        ingests a source (git, browser, file, teams,
                                        whatsapp-backup or a saved schema);
                                        without TARGET uses the configured targets;
                                        --gentle uses the ingest.background CPU/GPU limits
  ingest start <source|all> [TARGET...] the same in the background, with those limits;
                                        keeps going after the terminal closes
  ingest status|pause|resume|stop       follows, pauses (models kept in memory),
                                        continues or stops the ingestion
  timeline [--source S] [--all-authors] DATE [END_DATE]
                                        events of a day or range (DATE: YYYY-MM-DD,
                                        hoje, ontem); other authors' commits only with
                                        --all-authors
  timeline --around UID [--window 2h]   the event and what happened up to 2h before and after
  ask [--source S] [--from D] [--to D] [--no-filters] [--json] QUESTION
                                        natural-language question, in English or
                                        Portuguese (also about tasks: "which tasks did I finish?")
  tasks [--all] [DATE] [END_DATE]       tasks worked on (task links); PR opened means a
                                        visit to its creation page; approval/merge unknown
  reindex                               recomputes vectors with the configured
                                        embedding model (after changing it); resumable
  reindex --captions                    describes again the images whose description
                                        another model or prompt wrote; resumable
  compact                               returns the space of deleted vectors to the
                                        disk (no embedding recomputed); needs free
                                        space for a copy of the vectors
  encrypt [--yes]                       encrypts the database with a key kept in the
                                        system's vault (or a passphrase) and shows
                                        the recovery key
  decrypt [--yes]                       puts the database back in plain text and
                                        deletes the key
  forget <source> | --uid UID           deletes a source's events or one event;
  forget --match TEXT [--source S]      finds events; --yes confirms in scripts
                                        (also accepts --from D --to D);
                                        what left the source (e.g. an expired Teams
                                        cache) does not come back
  forget --person NAME                  deletes everything about a person (what they
                                        sent, mentions, chats with only them), after listing
  forget --backups                      deletes the database copies (before migrations),
                                        which still hold what was forgotten
  export [--source S] [--from D --to D] everything cade keeps, as JSON Lines
                                        (one event per line, with what was derived);
                                        --person NAME: only what is about them
  teams-schema DIR...                   structure (no values) of a Chrome IndexedDB,
                                        to design the Teams ingestor
  schema-discover --name N [--force] [--print] DIR
                                        generates with the local model a schema to
                                        ingest an application's browser storage
                                        (IndexedDB); --rollback --name N restores
                                        the previous revision; formerly
                                        idb-discover, which still works
  schema-check [--update [--rekey]] [N...]
                                        checks the applications still match their
                                        schemas; --update regenerates what changed;
                                        --rekey accepts a schema that changes UIDs and
                                        renames indexed events (after a copy);
                                        formerly idb-check, which still works
  version                               version, commit, date and build type (CPU/CUDA)
  help                                  shows this help

Examples:
  cade ingest git ~/src/my-project
  cade ingest browser ~/.mozilla/firefox/xyz.default/places.sqlite
  cade ingest all
  cade ingest start file
  cade timeline ontem
  cade timeline 2026-09-01 2026-09-07
  cade ask --source browser --from 2026-09-19 "what did I search about sqlite?"

Labels follow the locale, or ui.language in the config; answers follow
the question's language. Flags of each command (before or after the
arguments): cade <command> -h
`
