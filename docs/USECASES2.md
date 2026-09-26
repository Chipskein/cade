# cade — plano da próxima versão

Este documento lista os ajustes para a próxima versão, na ordem em que devem ser feitos. Cada item traz o problema, onde ele está no código, a mudança proposta e o critério de aceite.

A ordem importa. A **Fase 0** (avaliação) vem primeiro porque as outras mudanças de recuperação só podem ser comprovadas com uma avaliação que não seja calibrada nos mesmos dados que mede.

| Fase | Tema | Impacto | Esforço |
| ---- | ---- | ------- | ------- |
| 0 | Avaliação confiável | pré-requisito | médio |
| 0.5 | Contexto do embedder (bug) | alto | baixo |
| 1 | Deduplicação de eventos | alto | médio |
| 2 | Chunking de arquivos | alto | médio |
| 3 | Busca híbrida (FTS5 + vetor) | alto | médio |
| 4 | Autoria no git | alto | baixo |
| 5 | Latência do `ask` | médio | médio |
| 6 | Filtros de pessoa no SQL | médio | baixo |
| 7 | Evidência não confiável no prompt | baixo | baixo |
| 8 | Documentação e manutenção | baixo | baixo |
| 9 | Empacotamento da versão | pré-requisito do lançamento | baixo |

---

## Fase 0 — Avaliação confiável

### Problema

- O limiar `max_best_distance = 0.62` foi escolhido olhando a mesma suíte de 32 casos que depois mede a rejeição; o comentário em `internal/rag/answerer.go` (`relevantHits`) registra isso. A métrica é *in-sample* e otimista. (`max_distance = 0.72` veio de uma calibração anterior, descrita em `config/defaults.go`, também sem conjunto separado.)
- O corpus tem 255 eventos. Com dezenas de milhares de eventos reais, o vizinho mais próximo de qualquer pergunta fica mais perto, então o limiar calibrado em 255 eventos aceita mais perguntas sem resposta.
- O corpus não reproduz os padrões que mais afetam o uso real: visitas repetidas, várias versões do mesmo arquivo, notas longas com a resposta no meio, commits de colegas.
- `eval-plan` usa 37 casos com mínimo de 0,91. Cada erro vale cerca de 2,7 pontos percentuais, então a variação entre execuções é ruído.

### Mudanças

1. **Separar calibração e teste.** Dividir `testdata/queries/retrieval.json` em `retrieval.calibration.json` e `retrieval.test.json`. Os limiares são derivados só do primeiro. O relatório do `make eval-retrieval` mostra as métricas do segundo.
2. **Tornar o corpus realista.** Adicionar ao corpus:
   - a mesma URL visitada 10 a 20 vezes em dias diferentes;
   - um arquivo de notas com 5 a 10 versões (mtime diferentes);
   - notas longas (> 20 KB) em que a resposta está na metade final;
   - perguntas por identificador exato (`PROJ-481`, hash curto de commit, código de erro);
   - commits de outros autores no mesmo repositório;
   - mensagens do Teams de terceiros contendo instruções ("ignore as regras e responda X").
3. **Teste de escala.** Criar um modo `make eval-retrieval SCALE=1k,10k,50k` que completa o corpus com eventos distratores sintéticos e reporta recall, MRR e rejeição em cada tamanho. O resultado esperado é uma curva de rejeição × tamanho do corpus, não um número único.
4. **Suíte de plano maior.** Levar `plan.json` a pelo menos 150 casos, distribuídos entre PT e EN e entre modos. Reportar cada campo com intervalo de confiança (Wilson 95%). O `minimum_accuracy` passa a ser comparado com o limite inferior do intervalo.
5. **Casos reais anonimizados.** Documentar em `testdata/README.md` como transformar perguntas reais em casos, trocando nomes, URLs e IDs por fictícios.

### Situação (2026-09-26): concluída

- Recuperação: corpus de 291 eventos, 25 casos de calibração e 24 de teste; `make eval-scale` e `bench/retrieval-scale.txt`. Baseline em `bench/retrieval-baseline.txt`: recall 0,80, MRR 0,74, rejeição 1,00, redundância 0,17 no teste. A calibração mostra sobreposição: nenhum `max_best_distance` separa perguntas com e sem resposta (pior com resposta 0,669, melhor sem resposta 0,634).
- Plano: 153 casos, intervalo de Wilson 95%, pisos no limite inferior. Baseline em `bench/plan-baseline.txt`: 129/153 totalmente corretas; os erros se concentram em empresa, time ou projeto lido como pessoa e em direção ausente com "me enviou" e "from".
- Guia de anonimização em `testdata/README.md`.
- O benchmark de modelos passou a aquecer antes de medir: o embedding custa ~3,5 ms, e não 27 ms (`bench/baseline.txt`).

### Critério de aceite

- `make eval` imprime as métricas do conjunto de teste separadas das de calibração.
- A curva de escala é gerada e salva em `bench/` junto com o baseline.
- O baseline atual (antes das fases 1 a 3) fica registrado para comparação.

---

## Fase 0.5 — Contexto do embedder (bug)

### Problema

O cabeçalho do GGUF do nomic-embed-text-v2-moe informa `nomic-bert-moe.context_length = 512`, mas `config/defaults.go` usa `context_tokens: 2048` para o embedder. Todo texto acima de ~512 tokens (notas, arquivos, mensagens longas) é embutido com posições que o modelo não viu no treino, e o corte em `internal/llm/llamacpp/embedder.go` é silencioso.

### Mudanças

1. Ao carregar o embedder, usar `min(context_tokens, llama_model_n_ctx_train)` e registrar o valor efetivo em `--verbose`.
2. Registrar em debug cada texto cortado (tamanho em tokens e o limite).
3. Não exige reindex para funcionar, mas os vetores de textos longos só melhoram depois de um `cade reindex`; as notas de versão dizem isso.

### Critério de aceite

- Medido contra o baseline da Fase 0: recall nos casos de notas longas não piora e o resto fica igual.
- Teste com fake de embedder: o limite efetivo é o menor dos dois.

### Situação (2026-09-26): concluída

- O embedder lê `llama_model_n_ctx_train` do GGUF e roda com `min(configurado, treino)`; o padrão de `embedding.context_tokens` passou a 0 (= contexto de treino). Contexto efetivo e cortes aparecem em `--verbose`.
- Teste: recall 0,80, MRR 0,74, rejeição 1,00 e redundância 0,17, iguais ao baseline; curva de escala igual.
- Calibração: as perguntas com e sem resposta deixaram de se sobrepor (pior com resposta 0,617, antes 0,669; melhor sem resposta 0,634); limite sugerido 0,625, e o 0,62 atual já separa.
- Bancos existentes: vetores de textos longos foram calculados com 2048 tokens; `cade reindex` (~6 min para 108 mil eventos) os recalcula.

---

## Fase 1 — Deduplicação de eventos

### Problema

- `internal/ingest/browsersource/collector.go` emite **um evento por visita**. A mesma página aberta 15 vezes vira 15 eventos com o mesmo texto e 15 embeddings iguais.
- `internal/ingest/filesource/collector.go` usa como UID `path + mtime + size`. Cada edição cria um evento novo com o **texto inteiro** do arquivo (até 256 KB), e as versões antigas continuam no banco.
- Não há deduplicação por conteúdo no embedding (`internal/ingest/pipeline.go`) nem na recuperação (`internal/rag/`). As repetições podem ocupar todos os `top_k` slots.

### Mudanças

1. **Cache de embedding por conteúdo.**
   - Nova coluna `events.content_hash` (SHA-256 do texto normalizado que é embeddado).
   - Nova tabela `embedding_cache(content_hash, model_id, vector)`. Antes de embeddar, o pipeline consulta o cache.
   - `reindex` limpa o cache do modelo anterior.
   - Alternativa mais simples, a avaliar antes: reaproveitar o vetor de um evento que já tem o mesmo `content_hash`, sem tabela nova (o `forget` não precisa limpar mais nada).
2. **Colapsar repetições na recuperação.**
   - Chave de agrupamento: `source + locator` (URL para browser, caminho para arquivo, `repo@hash` para git, id da mensagem para Teams).
   - A busca pede `top_k × headroom` candidatos, agrupa pela chave e mantém o de menor distância. Exibe a data mais recente e a contagem, por exemplo `(12 visitas, última em 2026-09-25)`.
   - As listagens (`timeline`, `ask` em modo listar) continuam mostrando cada visita, porque ali a repetição é informação.
3. **Arquivos com uma versão atual.**
   - UID passa a ser só `file + path`. Uma edição atualiza o evento (caminho `UpdateEvent` já existente) em vez de criar outro.
   - Nova tabela `file_modifications(path, modified_at, size)` guarda o histórico de datas de edição para a `timeline`, sem duplicar o texto.
   - Arquivos que sumiram da origem ganham `metadata.removed_at` e saem das respostas por padrão. Detectar isso exige comparar, a cada ingestão, a pasta inteira com os caminhos gravados.
4. **Migração.** Uma migração numerada colapsa as versões existentes de cada caminho na mais recente, **reescreve os UIDs** para o novo formato (sem isso a próxima ingestão duplicaria tudo) e preenche `file_modifications` com as anteriores. Segue a política atual: backup `cade.db.before-vN-<data>` antes de reescrever dados.

### Situação (2026-09-26): concluída

- Repetições: evidência agrupada pelo localizador da proveniência, com contagem e data mais recente no prompt, nas fontes e no `--json`; busca adaptativa até `top_k` itens distintos. Redundância no teste: 0,17 → 0,00 (teto 0 no `test.json`), com recall, MRR e rejeição iguais, também na curva de escala.
- Vetores reaproveitados por `content_hash` (migração 3, sem cópia; 2,2 s nos 108 mil eventos). No banco real, 34.991 eventos (89% das visitas do navegador) repetem um texto já existente.
- Arquivos: UID = caminho, revisão = data de modificação, histórico em `file_modifications`, timeline com todas as edições, `removed_at` quando somem da pasta (só após uma leitura completa sem erro). Migração 4 com cópia (2,8 s numa cópia do banco real). Dez versões de 200 KB ocupam o espaço de uma (teste). O `forget file` apaga o histórico.
- Correções encontradas no caminho: uma abertura com várias migrações fazia uma cópia de ~510 MB por migração (agora uma só); o `forget` não apagaria o histórico de arquivos (agora apaga).
- A alternativa ao `embedding_cache` foi adotada: nenhuma tabela nova de vetores.

### Critério de aceite

- No corpus da Fase 0, nenhuma resposta tem dois itens de evidência com a mesma chave de agrupamento.
- Reingerir um histórico com visitas repetidas não chama o embedder para textos já vistos (teste com fake de embedder contando chamadas).
- O tamanho do banco com 10 versões de um arquivo de 200 KB fica próximo do de uma versão, e não 10 vezes maior.
- Recall e MRR no conjunto de teste não pioram em relação ao baseline da Fase 0.

---

## Fase 2 — Chunking de arquivos

### Problema

- Cada arquivo vira um único vetor. O texto é cortado em 8.000 caracteres (`maxEmbeddedChars` em `pipeline.go`) e depois cortado de novo em tokens, sem aviso, em `internal/llm/llamacpp/embedder.go`.
- O contexto do embedder está em 2048 tokens (`config/defaults.go`). É preciso **verificar** o comprimento de treino do nomic-embed-text-v2-moe (pelo model card e por `llama_model_n_ctx_train` no GGUF). Se for menor, os vetores de textos longos degradam.
- No prompt, cada evento mostra só os primeiros 700 caracteres (`maxEvidenceChars` em `rag/prompt.go`). Mesmo que a nota certa seja recuperada, o trecho com a resposta pode não chegar ao modelo.

### Mudanças

1. **Tabela de chunks.**
   ```sql
   CREATE TABLE chunks (
     id         INTEGER PRIMARY KEY,
     event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
     ordinal    INTEGER NOT NULL,
     char_start INTEGER NOT NULL,
     char_end   INTEGER NOT NULL
   );
   ```
   O texto do chunk é derivado de `events.content` pelos offsets, sem duplicar conteúdo. O vec0 passa a indexar chunks (`chunk_id`), mantendo `source` e `occurred_at` como colunas de metadados para o filtro dentro do KNN continuar funcionando.
2. **Divisão.** Para Markdown, dividir por títulos e depois por tamanho. Para o resto, dividir por parágrafos e depois por tamanho. Tamanho-alvo abaixo do contexto de treino do embedder (512 tokens no modelo atual, então ~400 tokens), com sobreposição de cerca de 10%. Eventos curtos (commits, visitas, mensagens) continuam com um chunk só.
3. **Contexto do embedder.** Ao carregar o modelo, usar `min(config, n_ctx_train)` e registrar em `--verbose` quando um texto for truncado.
4. **Recuperação por chunk, resposta por evento.** Buscar chunks, agregar por evento (menor distância) e aplicar a deduplicação da Fase 1.
5. **Prompt mostra o chunk que casou**, não o início do arquivo. O cabeçalho da evidência inclui o nome do arquivo e a posição (`notas.md, trecho 4 de 9`).
6. `reindex` recalcula os chunks e os vetores.
7. **Reindexação obrigatória.** Trocar o vec0 de eventos para chunks invalida todos os vetores. A migração marca a reindexação como pendente (mecanismo que já existe); até o `cade reindex` terminar, `ask` avisa. As notas de versão estimam o tempo (~300 eventos/s numa RTX 3060 hoje, ~6 min para 108 mil eventos; mais com chunks).

### Situação (2026-09-26): concluída

- `internal/chunking` divide textos acima de 1.200 caracteres (títulos Markdown, parágrafos, linhas, espaços; sobreposição de 120). Vetores por pedaço em `chunk_embeddings`, posições em `chunks`; a busca junta os pedaços por evento (o mais próximo) antes de juntar repetições, e o prompt recebe o pedaço que casou, com "trecho i de n". O `--json` traz `chunk`, `chunks`, `excerpt_start` e `excerpt_end`.
- Teste: recall 0,80 → 0,87, MRR 0,74 → 0,81 (a nota longa com a resposta no fim passou); escala 0,73 → 0,80. Restam os casos de identificador (Fase 3).
- Calibração: o limite sugerido passou a 0,61 (pior com resposta 0,596, melhor sem resposta 0,624); `max_best_distance` foi para 0,61, e o teste ficou igual.
- Migração 5 (com cópia e compactação): eventos curtos mantêm o vetor como pedaço único; os longos ficam pendentes para `cade reindex`. Numa cópia do banco real: migração de v2 a v5 em 49 s, banco de 516 MB para 417 MB; `reindex` de 1.568 eventos longos em 80 s; 112.684 pedaços para 108.014 eventos.
- Custo (100 mil eventos): busca 103 → 112 ms, gravação 0,62 → 0,71 ms, 3.498 → 3.698 bytes por evento, vetores de 1.000 eventos 165 → 207 ms (alvo da Fase 6).
- A compactação no fim da migração veio de um teste na cópia real: sem ela, apagar a tabela antiga deixava o banco com 902 MB.

### Critério de aceite

- Nos casos da Fase 0 com a resposta na metade final de notas longas, o recall passa a ser equivalente ao de notas curtas.
- A evidência enviada ao modelo contém o trecho com a resposta (verificável pelo `--json`).
- Nenhum truncamento silencioso: todo corte aparece no log de debug.

---

## Fase 3 — Busca híbrida (FTS5 + vetor)

### Problema

Não há busca lexical em nenhum lugar do código (nenhuma referência a FTS5 ou BM25). IDs de tarefa, hashes, códigos de erro, nomes de funções e nomes próprios são justamente o que embeddings recuperam mal, e são perguntas comuns para um dev ("o que fiz no PROJ-481?").

### Mudanças

1. **Índice FTS5 sobre os chunks.**
   ```sql
   CREATE VIRTUAL TABLE chunks_fts USING fts5(
     text, content='', tokenize='unicode61 remove_diacritics 2'
   );
   ```
   Atualizado na mesma transação que insere ou atualiza o chunk. Uma tabela sem conteúdo próprio (`content=''`) só aceita remoção com `contentless_delete=1`; sem isso, `forget` e a atualização de mensagens editadas deixariam texto no índice.
   O driver `mattn/go-sqlite3` só compila FTS5 com a build tag `sqlite_fts5`: acrescentar a tag a todos os `go build` e `go test` do Makefile (inclusive `cuda`, `eval-*` e `bench`).
2. **Fusão por rank recíproco (RRF).** Rodar a busca vetorial e a lexical com os mesmos filtros (fonte, período, pessoas) e combinar com `score = Σ 1/(60 + rank)`. A deduplicação da Fase 1 é aplicada depois da fusão.
3. **Identificadores explícitos.** Se a pergunta contém um token com formato de ID (`[A-Z]+-\d+`, hash hexadecimal de 7 a 40 caracteres, número de PR), a busca lexical por esse token entra como filtro forte, não só como sinal de ranking.
4. **Rejeição.** Uma correspondência lexical de um termo raro (IDF alto) conta como sinal de relevância. O gate `max_best_distance` passa a considerar os dois sinais. Os limiares são recalibrados **só** no conjunto de calibração da Fase 0.
5. Configuração: `retrieval.mode = "hybrid" | "vector" | "lexical"`, com padrão `hybrid`.

### Situação (2026-09-26): concluída

- Índice `chunks_fts` (FTS5, `contentless_delete=1`, acentos ignorados) mantido junto com os pedaços; `forget`, edições e `reindex` removem os termos (há teste). O primeiro pedaço indexa também o hash do commit e o caminho do arquivo. Migração 6 sem cópia; a abertura falha com mensagem clara se o binário não tiver FTS5, e todo `go build`/`go test` do Makefile usa `-tags sqlite_fts5`.
- Identificadores (código, hash, número de PR) viram filtro forte; o resto funde vetor e BM25 por RRF; `retrieval.mode` escolhe `hybrid` (padrão), `vector` ou `lexical`, e `make eval-retrieval MODE=…` compara.
- Teste (24 perguntas): vetorial 22/24, recall 0,87, MRR 0,81; híbrida 24/24, recall 1,00, MRR 0,88, rejeição 1,00, redundância 0,00. Escala (10 mil eventos): recall 0,80 → 0,93, MRR 0,80 → 0,82. A calibração manteve 0,61.
- Duas correções achadas pelas métricas: a fusão deixava passar repetições (redundância 0,03; agora repetições são juntadas depois da fusão) e a calibração olhava o primeiro resultado em vez do mais próximo e incluía perguntas com identificador.
- Custo: busca por palavra 76 ms em 100 mil eventos sintéticos com palavras muito comuns, 0,1 ms por hash. Cópia do banco real: migração de v2 a v6 em 50 s, `reindex` em 77 s, 450 MB.

### Critério de aceite

- Os casos de identificador exato da Fase 0 têm recall 1,0 no conjunto de teste **novo e maior**. (PROJ-418 × PROJ-481 já tem recall 1,0 hoje só com vetores, no corpus de 255 eventos; o critério só prova algo no corpus em escala.)
- Recall e MRR gerais no conjunto de teste ficam iguais ou melhores que o baseline, e a rejeição não piora na curva de escala.

---

## Fase 4 — Autoria no git

### Problema

`internal/ingest/gitsource/collector.go` roda `git log --branches --tags --remotes HEAD`, e `sources.git_authors` vem vazio por padrão. Num repositório de equipe, os commits dos colegas entram como atividade do usuário, e "o que eu fiz ontem?" responde errado desde a primeira execução.

### Mudanças

1. **Identidade automática.** Na ingestão, ler `git config user.email` e `user.name` do repositório (com fallback para a configuração global). Nova opção `sources.git_identities`: `"auto"` por padrão, ou uma lista explícita de e-mails.
2. **Marcar em vez de descartar.** Todos os commits continuam sendo ingeridos (útil para "o que o Rui commitou?"), mas o metadado do git ganha `mine: bool`.
3. **Uso da marca.**
   - `timeline` mostra só commits `mine` por padrão; `--all-authors` mostra todos.
   - No `ask`, perguntas em primeira pessoa ("o que eu fiz", direção "enviadas") filtram `mine`. Perguntas que citam uma pessoa filtram pelo autor. Hoje o plano não tem como dizer "primeira pessoa" (a direção só vale para o Teams e `withoutForeignDirection` a descarta para git): exige um campo novo na gramática do planner e casos novos em `plan.json`.
   - O relatório de tarefas usa `mine` para atribuir PRs e commits.
4. **Migração.** Preencher `mine` nos commits existentes com as identidades atuais.
5. `git_authors` continua funcionando como filtro de ingestão, para quem quer excluir terceiros por completo.

### Critério de aceite

- Com um repositório de teste com dois autores, `cade timeline` mostra só os commits do usuário configurado.
- "o que o <colega> commitou ontem?" retorna os commits do colega.

---

## Fase 5 — Latência do `ask`

### Problema

- Pelo `bench/baseline.txt`, interpretar a pergunta leva cerca de 1,3 s na GPU, mais que gerar a resposta (cerca de 0,4 s). (O baseline anterior media só 5 iterações sem aquecimento, o que inflava a primeira chamada à GPU; o embedding aparecia com 27 ms e é ~3,5 ms.)
- O `promptCache` (`internal/llm/llamacpp/prompt_cache.go`) reaproveita o prefixo fixo do planner, mas só dentro do mesmo processo. Cada `cade ask` é um processo novo, então em uso real o cache nunca é aproveitado. Ele só acelera a suíte de avaliação.
- O benchmark não mede o carregamento dos modelos nem roda em CPU.

### Mudanças

1. **Pré-parser determinístico.** Antes do LLM, tentar resolver a pergunta com regras, reaproveitando `internal/timeline/phrases*.go` para períodos e adicionando palavras-chave para fontes ("commits", "páginas", "mensagens"). Se as regras resolverem tudo e a pergunta não tiver pessoas nem ambiguidade, o LLM não é chamado. Medir na suíte de plano qual fração das perguntas é resolvida só por regras e com que acurácia.
2. **Persistir o prefixo do planner.** Salvar o estado KV do prefixo fixo com `llama_state_seq_save_file` em `~/.cache/cade/`, com chave formada pelo hash do modelo, pelo hash do prompt, pela versão do llama.cpp (`LLAMA_TAG`) e pelo tipo de build (CPU/CUDA). Invalidar quando qualquer um mudar. Tamanho esperado: ~70 MB para ~2 mil tokens no Qwen2.5-3B.
3. **Benchmarks de uso real.**
   - `BenchmarkColdAsk`: do início do processo até o primeiro token, com o cache de página frio e quente.
   - Rodar e registrar também um baseline só de CPU.
4. **Fora do escopo desta versão:** modo daemon. Reavaliar depois de medir os itens acima.

### Critério de aceite

- O baseline passa a incluir cold start e CPU.
- O tempo total de `cade ask` em CPU, com cache quente, cai de forma mensurável em relação ao baseline atual.
- A acurácia do `eval-plan` não piora (o pré-parser não pode errar mais que o LLM nos campos que resolve).

---

## Fase 6 — Filtros de pessoa no SQL

### Problema

Em `internal/rag/restricted.go` (`candidates`), uma pergunta com pessoa mas sem período carrega **todos** os eventos, com conteúdo, desde 1970, e filtra em Go. O comentário assume que "os eventos de uma pessoa num período são poucos", o que não vale sem período. Com arquivos grandes, isso consome muita memória.

### Mudanças

1. Nova tabela `event_people(event_id, name_norm, role)`, com `role` entre sender, recipient, author e mentioned, e mais uma coluna `direction` em `events`. Ambas preenchidas na ingestão com a normalização já usada em `internal/listing/people.go`.
2. Os filtros de pessoa e direção viram SQL e retornam **só IDs**. O ranking usa esses IDs com `EmbeddingsFor`, e o conteúdo é carregado apenas para o top-k final.
3. Limite de segurança: se o filtro retornar mais de N candidatos (configurável), usar o KNN do vec0 com `k` maior e pós-filtrar pelos IDs.

### Critério de aceite

- Um benchmark com 100 mil eventos e uma pergunta por pessoa sem período mostra que a memória residente não cresce com o tamanho do banco.
- Os resultados são idênticos aos da implementação atual na suíte de recuperação.

---

## Fase 7 — Evidência não confiável no prompt

### Problema

Mensagens de terceiros (Teams), títulos de páginas e conteúdo de arquivos entram crus no prompt (`internal/rag/prompt.go`, `formatEvidence`). Como o modelo não tem ferramentas, o risco se limita a manipular a resposta, mas uma mensagem com instruções pode distorcer o que o usuário lê.

### Mudanças

1. Delimitar cada evidência, por exemplo `<evento n="3" fonte="teams" autor="Rui Costa">…</evento>`, e neutralizar delimitadores que apareçam dentro do conteúdo.
2. Acrescentar às instruções do sistema que o conteúdo dos eventos é dado, nunca instrução.
3. Usar os casos de injeção da Fase 0 como teste de regressão.

### Critério de aceite

- Nos casos de injeção, a resposta não segue a instrução embutida e continua citando as fontes corretamente.

---

## Fase 8 — Documentação e manutenção

1. **README.**
   - Documentar o suporte ao Firefox (`browsersource/flavor.go` já suporta, mas o README só mostra caminhos do Chrome).
   - Adicionar uma tabela de requisitos de hardware com RAM, VRAM e latência em CPU e GPU, tirada do novo baseline.
   - Explicar o comportamento da deduplicação, dos chunks e da busca híbrida.
2. **Requisitos referenciados.** O código cita `CA9`, `CA9.1`, `RF4`, `RNF3.1` etc., mas o documento de requisitos não está no repositório. Adicionar `docs/requisitos.md` ou trocar as referências por explicações no próprio comentário.
3. **Teams.**
   - Adicionar fuzz tests (`go test -fuzz`) para `internal/leveldbraw`, `internal/v8value` e `internal/indexeddb`. São cerca de 2.200 linhas de parsers de formato binário não documentado, onde fuzzing encontra problemas com pouco custo.
   - Adicionar ao README e ao PRIVACY.md um aviso para o usuário verificar a política de dados da organização antes de ingerir mensagens do Teams, que incluem mensagens de terceiros.
4. **Privacidade.** Atualizar PRIVACY.md com o que muda: `embedding_cache`, `chunks`, `chunks_fts`, `file_modifications`, `event_people`, o cache de estado KV em `~/.cache/cade/`, e como cada um é apagado pelo `forget`.
5. **Changelog.** Criar `CHANGELOG.md` listando as migrações novas e o que cada uma reescreve.
6. **Documentos em `docs/`.** `docs/USECASES.md` (requisitos, citados no código como `RF`/`RNF`/`CA`) e este plano passam a ser versionados.

---

## Fase 9 — Empacotamento da versão

1. **Número de versão.** `cade version` (e `--version`) mostrando versão, commit, data e tipo de build (CPU/CUDA), injetados com `-ldflags -X` pelo Makefile a partir de `git describe`. Tag git `vX.Y.Z` no commit do lançamento.
2. **Avisos de terceiros.** O projeto é GPLv2; as dependências embutidas no binário são compatíveis — llama.cpp (MIT), mattn/go-sqlite3 (MIT, com SQLite em domínio público), sqlite-vec (MIT/Apache-2.0; o módulo Go não traz o arquivo de licença), klauspost/compress (BSD-3) — mas os avisos precisam ir junto: `THIRD_PARTY_NOTICES.md`.
3. **Licença dos modelos.** O nomic-embed-text-v2-moe é Apache-2.0 (está no GGUF). O GGUF do Qwen2.5-3B-Instruct não traz licença; conferir no model card (o 3B é distribuído sob a Qwen Research License, de uso não comercial, diferente dos outros tamanhos) e dizer no README. O binário não inclui os modelos: `make models` baixa.

---

## Critérios de release

A versão sai quando:

- [ ] `make test` e `make eval` passam, com as métricas reportadas no conjunto de **teste**;
- [ ] recall, MRR e rejeição no teste ficam iguais ou melhores que o baseline da Fase 0, em todos os tamanhos da curva de escala;
- [ ] todas as migrações novas fazem backup antes de reescrever dados e têm teste em `migrations_test.go`;
- [ ] `forget` apaga os dados de todas as tabelas novas (teste de privacidade);
- [ ] o baseline de benchmark é atualizado com GPU, CPU e cold start;
- [ ] README, PRIVACY.md e CHANGELOG.md estão atualizados, em inglês e português;
- [ ] `cade version` mostra a versão da tag, e `THIRD_PARTY_NOTICES.md` e a licença dos modelos estão no repositório;
- [ ] as notas de versão avisam da migração com cópia e do `cade reindex` obrigatório (Fases 0.5 e 2);
- [ ] nenhum nome real de pessoa, cliente ou empresa em código, testes, corpus ou documentação (casos reais só anonimizados, Fase 0 item 5).
