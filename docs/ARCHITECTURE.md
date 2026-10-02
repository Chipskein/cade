<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade — arquitetura

Como o código está organizado e qual componente chama qual, no estado da v0.0.0. Os diagramas são em Mermaid e seguem os nomes reais de pacotes, tipos e funções, para servirem de mapa ao ler o código. O que muda na v0.1.0 está no [ROADMAP](ROADMAP.md).

## Índice

- [Como funciona (visao geral)](#como-funciona-visao-geral)
- [Pacotes](#pacotes)
- [Montagem: quem cria as implementações](#montagem-quem-cria-as-implementações)
- [`cade ingest`](#cade-ingest)
- [`cade ask`](#cade-ask)
- [Busca de evidências (`rag.Answerer.Retrieve`)](#busca-de-evidências-raganswererretrieve)
- [`cade timeline` e `cade tasks`](#cade-timeline-e-cade-tasks)
- [`cade reindex`](#cade-reindex)
- [Banco](#banco)

---

## Como funciona (visao geral)

Fluxo de alto nível para lembrar o caminho das fontes até as consultas:

```mermaid
flowchart LR
    subgraph Fontes
        git[Git]
        nav[Navegador]
        arq[Arquivos]
        teams[Teams · IndexedDB]
    end

    arq -->|imagens, opcional| visao[Descrição local<br/>Qwen3.5 + mmproj]
    visao --> ingest
    Fontes --> ingest[cade ingest<br/>normaliza + embedding]
    ingest --> db[(SQLite + sqlite-vec)]

    timeline[cade timeline] --> db
    tasks[cade tasks] --> relatorio[Tarefas e PRs<br/>por links de tarefa e PR]
    relatorio --> db

    ask[cade ask] --> plano[Interpreta a pergunta<br/>regras, ou LLM + gramática]
    plano -->|listar| filtro[Filtra no banco]
    plano -->|responder| busca[Filtra + busca vetorial]
    plano -->|tarefas| relatorio
    filtro --> db
    busca --> db
    busca --> llm[LLM local<br/>resposta com fontes]
```

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
        sources["ingest/gitsource<br/>ingest/browsersource<br/>ingest/filesource<br/>ingest/teamssource<br/>ingest/idbsource"]
        chunking["chunking"]
        indexeddb["indexeddb (Chromium)<br/>+ leveldbraw, snappyblock,<br/>v8value, filecopy<br/>firefoxidb + smclone (Firefox)"]
        idbmap["idbmap<br/>schema de IndexedDB,<br/>aplicação, deriva, histórico"]
        idbdiscovery["idbdiscovery<br/>schema pelo modelo local"]
        imagecaption["imagecaption<br/>descrição de imagens (fase 19)"]
        imagefile["imagefile<br/>png, jpeg, webp → RGB"]
        ingestrun["ingestrun<br/>estado e trava do ingest (#41)"]
        pacing["pacing<br/>descanso dos modelos (#41)"]
    end

    subgraph portas["Interfaces do núcleo"]
        llm["llm<br/>Embedder, Generator,<br/>StructuredGenerator, ImageDescriber"]
        storage["storage<br/>EventStore, EmbeddingIndex,<br/>ImageCaptionIndex"]
        event["event<br/>Event"]
    end

    subgraph impl["Implementações"]
        llamacpp["llm/llamacpp<br/>(cgo, llama.cpp)"]
        sqlitestore["storage/sqlitestore<br/>(SQLite, sqlite-vec, FTS5)"]
        imagepreview["imagepreview<br/>prévia de imagem citada<br/>(programa chafa, opcional)"]
        procctl["procctl<br/>processo fora do terminal,<br/>parada e prioridade (#41)"]
    end

    main --> cli
    main --> sources
    main --> llamacpp
    main --> sqlitestore
    main --> config
    main --> imagepreview
    main --> procctl

    cli --> doctor
    cli --> discovery
    cli --> queryplan
    cli --> rag
    cli --> timeline
    cli --> tasks
    cli --> ingest
    cli --> imagecaption
    cli --> provenance
    cli --> ingestrun
    cli --> pacing
    cli --> idbdiscovery
    idbdiscovery --> idbmap
    idbdiscovery --> llm

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
    sources --> idbmap
    idbmap --> indexeddb
    ingest --> chunking
    ingest --> llm
    ingest --> storage
    imagecaption --> sources
    imagecaption --> imagefile
    imagecaption --> llm
    imagecaption --> storage
    pacing --> llm

    storage --> listing
    storage --> event
    tasks --> event

    imagepreview --> imagefile

    llamacpp -. implementa .-> llm
    sqlitestore -. implementa .-> storage
```

Pacotes que não aparecem: `textnorm` (normalização de texto, usada por quase todos), `rootfs` (sistema de arquivos para o `init` e o `doctor`), `idbschema` (o resumo sem valores do `cade teams-schema`, que a descoberta e a deriva também usam), `htmltext` (HTML de mensagem para texto, do Teams e do `html_text` dos schemas), `buildinfo` (`cade version`) e os de teste (`testfakes`, `testcheck`, `benchmarks`, `retrievalsuite`, `syntheticimage`, que desenha as imagens de teste, e `evalimages`, que abre o cache das descrições delas).

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
        loadDescriber["LoadImageDescriber → llamacpp gerador + mmproj<br/>(só o ingest com imagens)"]
        sourcesFn["Sources → sourceSpecs(cfg, captions)<br/>git, browser, file, teams<br/>+ uma fonte por schema de IndexedDB salvo"]
        loadConfig["LoadConfig / WriteConfig"]
        readIDB["ReadIndexedDB → readIndexedDB<br/>indexeddb (Chromium) ou firefoxidb (Firefox)"]
        schemaFiles["SchemaFiles → idbmap.OSSchemaFiles"]
    end

    toolkit --- toolkitfns
    run -->|"subcommands()"| cmds["runIngest · runAsk · runTimeline · runTasks<br/>runForget · runReindex · runInit · runDoctor<br/>runTeamsSchema · runIDBDiscover · runIDBCheck · runVersion"]
```

---

## `cade ingest`

Um `ingest` abre o banco, carrega só o modelo de embedding e passa cada coletor pelo mesmo `ingest.Pipeline`. Os coletores não sabem de banco nem de vetores: só emitem `event.Event`.

Com `sources.images` ligado e alvos da fonte `file`, uma primeira etapa (fase 19) descreve as imagens antes de o embedder carregar. O `imagecaption.Planner` percorre as pastas com o mesmo `filesource.Walk` do coletor, e o resultado (`ingest.ImageCaptions`, caminho → `event.Image`) vai para o coletor de arquivos, que usa a descrição como texto do evento. Daí em diante é o pipeline de sempre: máscara de segredos, pedaços, vetores, `chunks_fts`.

```mermaid
sequenceDiagram
    autonumber
    participant CLI as cli.describeImages
    participant PL as imagecaption.Planner
    participant S as storage.EventStore<br/>(ImageCaptionIndex)
    participant IF as imagefile
    participant V as llamacpp.ImageDescriber<br/>(gerador + mmproj)

    loop cada imagem das pastas (filesource.Walk)
        PL->>S: IsForgotten / StoredEvent(uid)
        alt mesma versão, já descrita ou ilegível
            PL-->>PL: pula sem ler o arquivo
        else nova, mudou ou pendente
            PL->>S: DescribedImage(sha256)
            alt mesmos bytes já descritos (movida, renomeada)
                S-->>PL: descrição reaproveitada
            else ainda cabe em max_images_per_run
                PL->>IF: Decode (limite de pixels, lado ≤ 1024)
                PL->>V: DescribeImage (carrega na 1ª vez)
                V-->>PL: descrição + texto visível
            else passou do limite
                PL-->>PL: pendente (próxima execução)
            end
        end
    end
    CLI->>V: Close (antes do LoadEmbedder)
```

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

Os coletores de IndexedDB têm um caminho próprio até o evento. O Chromium e o Firefox guardam o IndexedDB em formatos diferentes, mas os dois leitores entregam a mesma árvore `v8value.Value`, então o resto não sabe de que navegador o dado veio:

```mermaid
flowchart LR
    dir["pasta *.indexeddb.leveldb<br/>(Chromium)"] --> copy["filecopy<br/>cópia em /tmp (arquivos travados)"]
    fdir["pasta idb/*.sqlite<br/>(Firefox, Floorp)"] --> copy
    copy --> ldb["leveldbraw + snappyblock<br/>registros brutos"]
    ldb --> idb["indexeddb.ReadDirectory<br/>stores e registros"]
    idb --> v8["v8value<br/>valores serializados do V8"]
    copy --> fidb["firefoxidb<br/>SQLite + snappyblock"]
    fidb --> sm["smclone<br/>structured clone do SpiderMonkey"]
    v8 --> teams["teamssource.Collector<br/>mensagens, conversas, perfis"]
    v8 --> idbsrc["idbsource.Collector<br/>idbmap.Mapper + schema salvo"]
    sm --> teams
    sm --> idbsrc
    teams -->|"emit(event.Event)"| pipeline["ingest.Pipeline"]
    idbsrc -->|"emit(event.Event)"| pipeline
```

O schema de um aplicativo sai do `cade idb-discover`: o `idbdiscovery` monta um catálogo dos stores (caminhos, tipos e amostras mascaradas), pergunta ao modelo duas vezes sob gramáticas geradas a partir desses caminhos e aplica o schema proposto aos próprios registros antes de salvá-lo. O `cade idb-check` mede a deriva contra a impressão digital do schema e, com `--update`, regenera, compara (`idbmap.CompareSchemas`) e troca guardando a revisão anterior; com `--rekey`, renomeia os UIDs já indexados (`storage.EventStore.RekeyEvents`, com cópia do banco).

### Em segundo plano (`ingest start`, `status`, `pause`, `resume`, `stop`, `--gentle`)

O `cli` não faz nada disso direto: o `Toolkit.IngestRuns` traz o arquivo de estado e a trava (`ingestrun`), o controle de processos (`procctl`) e o relógio do `pacing`, e os testes trocam tudo por fakes.

```mermaid
sequenceDiagram
    autonumber
    participant U as cade ingest start
    participant PC as procctl.System
    participant F as cade (filho, setsid)
    participant L as ingestrun.FileLock
    participant ST as ingestrun.JSONStateFile
    participant M as modelos (pacing)

    U->>U: planIngest (fonte errada falha aqui)
    U->>L: Acquire + libera (outra ingestão? recusa com o pid)
    U->>PC: StartDetached(--config C ingest -- ARGS, CADE_INGEST_DETACHED=1, ingest.log)
    PC-->>F: novo processo, sem terminal, stdin /dev/null
    F->>L: Acquire (até o processo acabar)
    F->>PC: LowerPriority (nice 19, E/S ociosa, cada thread)
    F->>ST: Write(running, args, modo)
    loop cada imagem / evento
        F->>M: DescribeImage / Embed, depois descansa (busy_percent)
        F->>ST: etapa e linha de progresso (no máximo 1 vez por segundo)
    end
    F->>ST: finished · interrupted (SIGTERM, SIGHUP, Ctrl-C) · failed
```

O processo filho é um `cade ingest` comum com `config.WithBackgroundLimits()` (threads, `gpu_layers`, imagens por execução de `ingest.background`) e com o embedder e o descritor de imagens embrulhados em `pacing.PacedEmbedder` e `pacing.PacedDescriber`. `--gentle` faz o mesmo no próprio terminal. `status` lê o estado e confere se o pid ainda existe (o `doctor` mostra a mesma data); `pause` manda SIGSTOP e grava `paused`, já que o processo congelado não grava nada; `stop` manda SIGTERM e SIGCONT, que cancelam o contexto como o Ctrl-C mesmo numa execução pausada; `resume` manda SIGCONT a uma pausada ou roda de novo os argumentos gravados, no mesmo modo, e a deduplicação (RF1.5) pula o que já foi gravado.

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

Reaproveita o `ingest.Pipeline` sem coletor: só refaz os vetores. Com `--captions`, descreve de novo as imagens cuja descrição veio de outro modelo ou versão do prompt, em lotes de `ingest.max_images_per_run`: em cada lote, o `imagecaption.Recaptioner` descreve com o modelo de visão, que é liberado antes de o embedder carregar para `Pipeline.ReplaceStored` gravar o texto novo (com a máscara).

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
    events["events<br/>uid, fonte, data, título,<br/>texto, metadado, content_hash,<br/>direction; índice do image_sha256"]
    chunks["chunks<br/>por texto: fonte, content_hash,<br/>posições no texto"]
    vec["chunk_embeddings<br/>(sqlite-vec, int8)<br/>fonte, first_at, last_at"]
    fts["chunks_fts<br/>(FTS5)"]
    ids["event_identifiers_fts<br/>(FTS5) hash do commit,<br/>caminho do arquivo"]
    people["event_people<br/>nomes por evento"]
    files["file_modifications<br/>versões anteriores<br/>(data, tamanho)"]
    settings["store_settings<br/>embedding_model,<br/>embedding_dimensions"]

    events -->|"fonte + content_hash"| chunks
    chunks --> vec
    chunks --> fts
    events --> ids
    events --> people
    events --> files
```

- **Pedaços por texto (#67):** eventos com o mesmo texto e a mesma fonte dividem um conjunto de `chunks`, vetores e entradas no `chunks_fts`; o evento chega a ele pelo `content_hash`. O conjunto é gravado com o primeiro evento do texto e apagado com o último (`releaseText` em `chunks.go`).
- **Filtros na busca vetorial (CA9.1):** a fonte é uma coluna do vec0. A data não pode ser, porque os eventos de um texto podem estar anos distantes: o vec0 guarda a primeira e a última data deles (`first_at`, `last_at`), o KNN fica com os textos cujo intervalo cruza o período, e os eventos decidem. Se menos de k textos têm evento no período, o k aumenta até o limite do sqlite-vec (`search.go`). Cada texto achado vira um resultado por evento dele que passa nos filtros.
- **Identificadores por evento:** o hash de um commit e o caminho de um arquivo ficam no `event_identifiers_fts`, por evento; a busca por palavras junta os dois índices pelo BM25.
