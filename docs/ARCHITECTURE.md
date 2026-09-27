# cade — arquitetura

Como o código está organizado e qual componente chama qual, no estado da v0.0.0. Os diagramas são em Mermaid e seguem os nomes reais de pacotes, tipos e funções, para servirem de mapa ao ler o código. O que muda na v0.1.0 está no [ROADMAP](ROADMAP.md).

## Índice

- [Pacotes](#pacotes)
- [Montagem: quem cria as implementações](#montagem-quem-cria-as-implementações)
- [`cade ingest`](#cade-ingest)
- [`cade ask`](#cade-ask)
- [Busca de evidências (`rag.Answerer.Retrieve`)](#busca-de-evidências-raganswererretrieve)
- [`cade timeline` e `cade tasks`](#cade-timeline-e-cade-tasks)
- [`cade reindex`](#cade-reindex)
- [Banco](#banco)

---

## Pacotes

As setas são imports (`A --> B`: A usa B). O núcleo só conhece interfaces (`llm.Embedder`, `llm.Generator`, `storage.EventStore`); as implementações concretas (`llamacpp`, `sqlitestore`) só são importadas pelo `cmd/cade`.

```mermaid
flowchart TD
    main["cmd/cade<br/>main, sources"]

    subgraph interface["Interface"]
        cli["cli<br/>subcomandos, saída, idioma"]
        doctor["doctor"]
        discovery["discovery<br/>(cade init)"]
        config["config"]
    end

    subgraph consulta["Consulta"]
        queryplan["queryplan<br/>regras + planejador"]
        rag["rag<br/>busca e resposta"]
        timeline["timeline<br/>períodos e listagem"]
        tasks["tasks<br/>relatório de tarefas"]
        listing["listing<br/>critérios de pessoa/direção"]
        provenance["provenance<br/>origem de cada evento"]
    end

    subgraph ingestao["Ingestão"]
        ingest["ingest<br/>Pipeline, SourceSpec"]
        sources["ingest/gitsource<br/>ingest/browsersource<br/>ingest/filesource<br/>ingest/teamssource"]
        chunking["chunking"]
        indexeddb["indexeddb<br/>+ leveldbraw, snappyblock,<br/>v8value, filecopy"]
    end

    subgraph portas["Interfaces do núcleo"]
        llm["llm<br/>Embedder, Generator,<br/>StructuredGenerator"]
        storage["storage<br/>EventStore, EmbeddingIndex"]
        event["event<br/>Event"]
    end

    subgraph impl["Implementações"]
        llamacpp["llm/llamacpp<br/>(cgo, llama.cpp)"]
        sqlitestore["storage/sqlitestore<br/>(SQLite, sqlite-vec, FTS5)"]
    end

    main --> cli
    main --> sources
    main --> llamacpp
    main --> sqlitestore
    main --> config

    cli --> doctor
    cli --> discovery
    cli --> queryplan
    cli --> rag
    cli --> timeline
    cli --> tasks
    cli --> ingest
    cli --> provenance

    queryplan --> llm
    queryplan --> listing
    queryplan --> timeline
    rag --> queryplan
    rag --> storage
    rag --> llm
    rag --> chunking
    rag --> provenance
    timeline --> storage

    sources --> ingest
    sources --> indexeddb
    ingest --> chunking
    ingest --> llm
    ingest --> storage

    storage --> listing
    storage --> event
    tasks --> event

    llamacpp -. implementa .-> llm
    sqlitestore -. implementa .-> storage
```

Pacotes que não aparecem: `textnorm` (normalização de texto, usada por quase todos), `rootfs` (sistema de arquivos para o `init` e o `doctor`), `idbschema` (`cade teams-schema`), `buildinfo` (`cade version`) e os de teste (`testfakes`, `testcheck`, `benchmarks`, `retrievalsuite`).

---

## Montagem: quem cria as implementações

`main` monta um `cli.Toolkit` com funções que abrem o banco e carregam os modelos. O `cli` recebe só essas funções, então os testes trocam tudo por fakes (`internal/testfakes`) sem llama.cpp nem SQLite.

```mermaid
flowchart LR
    main["main()"] -->|"productionToolkit()"| toolkit["cli.Toolkit"]
    toolkit --> run["cli.Run(ctx, args, stdout, stderr, toolkit)"]

    subgraph toolkitfns["Campos do Toolkit"]
        openStore["OpenStore → sqlitestore.OpenWithHooks<br/>(migrações, cópia antes)"]
        loadEmbedder["LoadEmbedder → llamacpp embedder"]
        loadGenerator["LoadGenerator → llamacpp generator<br/>(estado do prompt salvo)"]
        sourcesFn["Sources → sourceSpecs(cfg)<br/>git, browser, file, teams"]
        loadConfig["LoadConfig / WriteConfig"]
        readIDB["ReadIndexedDB → indexeddb.ReadDirectory"]
    end

    toolkit --- toolkitfns
    run -->|"subcommands()"| cmds["runIngest · runAsk · runTimeline · runTasks<br/>runForget · runReindex · runInit · runDoctor<br/>runTeamsSchema · runVersion"]
```

---

## `cade ingest`

Um `ingest` abre o banco, carrega só o modelo de embedding e passa cada coletor pelo mesmo `ingest.Pipeline`. Os coletores não sabem de banco nem de vetores: só emitem `event.Event`.

```mermaid
sequenceDiagram
    autonumber
    participant CLI as cli.runIngest
    participant TK as Toolkit
    participant P as ingest.Pipeline
    participant C as EventCollector<br/>(git/browser/file/teams)
    participant CH as chunking
    participant E as llm.Embedder
    participant S as storage.EventStore

    CLI->>CLI: planIngestJobs(Sources(cfg), fonte, alvos)
    CLI->>TK: OpenStore (migrações pendentes rodam aqui)
    CLI->>TK: LoadEmbedder(cfg.Embedding)
    CLI->>P: NewPipeline(store, embedder, prefixo)
    loop cada alvo
        CLI->>C: spec.NewCollector(alvo)
        CLI->>P: Run(ctx, collector, progresso)
        P->>C: CollectEvents(ctx, emit)
        loop cada evento emitido
            C-->>P: emit(event)
            P->>S: StoredEvent(uid)
            alt já guardado e igual
                P-->>P: pula (sem embedding)
            else novo ou mudou na origem
                P->>S: StoredChunksForContent(texto)
                alt texto idêntico já embutido
                    S-->>P: pedaços com vetores reaproveitados
                else texto novo
                    P->>CH: pedaços de até 1.200 caracteres
                    loop cada pedaço
                        P->>E: Embed(prefixo + pedaço)
                    end
                end
                P->>S: SaveEvent ou UpdateEvent(evento, pedaços)
            end
        end
        opt AuthoredCollector (git)
            P->>S: marca os commits do usuário
        end
        opt SnapshotCollector (file)
            P->>S: marca arquivos que sumiram da pasta
        end
        P-->>CLI: Report (coletados, inseridos, atualizados…)
    end
```

O coletor do Teams tem um caminho próprio até o evento:

```mermaid
flowchart LR
    dir["pasta *.indexeddb.leveldb"] --> copy["filecopy<br/>cópia em /tmp (arquivos travados)"]
    copy --> ldb["leveldbraw + snappyblock<br/>registros brutos"]
    ldb --> idb["indexeddb.ReadDirectory<br/>stores e registros"]
    idb --> v8["v8value<br/>valores serializados do V8"]
    v8 --> teams["teamssource.Collector<br/>mensagens, conversas, perfis"]
    teams -->|"emit(event.Event)"| pipeline["ingest.Pipeline"]
```

---

## `cade ask`

`askWithStore` interpreta a pergunta e escolhe um de três caminhos. Os modelos são carregados sob demanda por `askModels`: uma listagem lida por regras não carrega nenhum.

```mermaid
sequenceDiagram
    autonumber
    participant CLI as cli.askWithStore
    participant QP as queryplan
    participant G as llm.StructuredGenerator
    participant M as askModels
    participant TL as timeline.Lister
    participant TB as tasks.Builder
    participant A as rag.Answerer
    participant GEN as llm.Generator

    CLI->>M: newAskModels (nada carregado ainda)
    CLI->>QP: PlanByRules(pergunta)
    alt regras cobrem a pergunta
        QP-->>CLI: Plan
    else precisa do modelo
        CLI->>M: loadedGenerator()
        CLI->>QP: NewPlanner(G).Plan(pergunta)
        QP->>G: GenerateStructured(mensagens, gramática GBNF)
        G-->>QP: JSON
        QP-->>CLI: Plan (passado por guardPlan)
    end
    CLI->>QP: Resolve(pergunta, plan, flags, agora) → Query
    CLI->>CLI: announceQuery ("Entendi: …" no stderr)

    alt Query.Mode = listar
        CLI->>TL: List(dias, fonte)
        CLI->>CLI: Criteria.Apply (pessoas, direção)
        opt tem assunto
            CLI->>A: FilterByTopic(assunto, eventos)
        end
        CLI-->>CLI: renderTimeline
    else Query.Mode = tarefas
        CLI->>TB: Build(eventos do período, histórico)
        CLI-->>CLI: relatório de tarefas
    else responder
        CLI->>M: answerer() (carrega o embedder)
        CLI->>A: Answer(query, observer)
        A->>A: Retrieve (ver abaixo)
        alt nenhuma evidência
            A-->>CLI: Answer vazio ("não encontrei", sem chamar o modelo)
        else evidências
            A->>GEN: Generate(buildPrompt(pergunta, evidências, hoje))
            GEN-->>A: texto em streaming (tokens no terminal)
            A->>A: citedIndexes (citações [n] e as que não existem)
            A-->>CLI: Answer
        end
        CLI-->>CLI: render (resposta + fontes citadas) ou --json
    end
```

---

## Busca de evidências (`rag.Answerer.Retrieve`)

O caminho depende de a pergunta ter critérios exatos (pessoas, direção), um identificador (`PROJ-481`, hash) ou nenhum dos dois.

```mermaid
flowchart TD
    start["Retrieve(query)"] --> embed["embedQuery(searchText)<br/>llm.Embedder"]
    embed --> criteria{"Criteria<br/>(pessoas, direção)?"}

    criteria -->|sim| resolve["resolveFilter<br/>storage.ResolveCriteria<br/>(event_people)"]
    resolve --> many{"mais que<br/>max_filtered_events?"}
    many -->|não| rankAmong["rankAmong<br/>EventsMatching + distância<br/>de cada pedaço"]
    many -->|sim| searchAmong["searchAmong<br/>busca vetorial restrita"]
    rankAmong --> topk1["top_k"]
    searchAmong --> topk1

    criteria -->|não| ident{"identificador<br/>na pergunta?"}
    ident -->|"sim, e achou"| lexId["searchLexical<br/>(FTS5, chunks_fts)"]
    ident -->|não| hybrid

    subgraph hybrid["search (modo híbrido)"]
        lex["searchLexical<br/>palavras da pergunta"]
        vec["searchDistinct<br/>SearchSimilar (sqlite-vec)"]
        fuse["fuseRankings<br/>reciprocal rank fusion"]
        repeats["collapseRepeats<br/>visitas repetidas = 1 fonte"]
        lex --> fuse
        vec --> fuse
        fuse --> repeats
    end

    hybrid --> scoped{"período ou fonte<br/>na pergunta?"}
    scoped -->|sim| out["evidências"]
    scoped -->|não| gates["relevantHits<br/>max_best_distance, max_distance"]
    gates --> out
    lexId --> out
    topk1 --> out
```

Depois disso, `generate` monta o prompt (`rag/prompt.go`): eventos que dão ordens ao assistente são marcados como não confiáveis e vão sem o texto (`rag/injection.go`), e mensagens sem conteúdo ("ok", "valeu") já ficaram de fora (`rag/chatter.go`).

---

## `cade timeline` e `cade tasks`

Nenhum dos dois carrega modelo.

```mermaid
flowchart LR
    subgraph tl["cade timeline"]
        t1["runTimeline"] --> t2["timeline.ParseDayRange"]
        t2 --> t3["timeline.Lister.List<br/>(+ versões de arquivo)"]
        t3 --> t4["store.EventsBetween"]
        t3 --> t5["renderTimeline"]
    end

    subgraph tk["cade tasks"]
        k1["runTasks"] --> k2["buildTaskReport"]
        k2 --> k3["store.EventsBetween<br/>período + histórico para PRs"]
        k3 --> k4["tasks.NewBuilder(task_url_patterns).Build"]
        k4 --> k5["render do relatório"]
    end
```

---

## `cade reindex`

Reaproveita o `ingest.Pipeline` sem coletor: só refaz os vetores.

```mermaid
sequenceDiagram
    participant CLI as cli.runReindex
    participant P as ingest.Pipeline
    participant IX as storage.EmbeddingIndex
    participant E as llm.Embedder

    CLI->>P: NewPipeline(store, embedder, prefixo)
    CLI->>P: Reindex(index, modelo)
    P->>IX: EmbeddingModel()
    alt modelo mudou ou rebuild pendente
        P->>IX: StartReindex(modelo) (apaga os vetores)
    end
    loop lotes
        P->>IX: EventsWithoutEmbedding(lote)
        P->>E: Embed(cada pedaço)
        P->>IX: SaveEmbeddings
    end
```

---

## Banco

Um arquivo SQLite (`~/.local/share/cade/cade.db`), com as extensões sqlite-vec e FTS5. As migrações ficam em `internal/storage/sqlitestore/migrations.go` e estão listadas no [CHANGELOG](../CHANGELOG.pt-BR.md#migrações).

```mermaid
flowchart LR
    events["events<br/>uid, fonte, data, título,<br/>texto, metadado, content_hash,<br/>direction"]
    chunks["chunks<br/>posições no texto"]
    vec["chunk_embeddings<br/>(sqlite-vec)"]
    fts["chunks_fts<br/>(FTS5)"]
    people["event_people<br/>nomes por evento"]
    files["file_modifications<br/>versões anteriores<br/>(data, tamanho)"]
    settings["store_settings<br/>embedding_model,<br/>embedding_dimensions"]

    events --> chunks
    chunks --> vec
    chunks --> fts
    events --> people
    events --> files
```
