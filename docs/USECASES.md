# Especificação — CLI de Histórico Pessoal

> Ferramenta CLI local que agrega a atividade pessoal de múltiplas fontes e responde consultas por timeline e por busca semântica. 100% local por requisito de privacidade.

---

## Índice

1. [Visão geral](#1-visão-geral)
2. [Requisitos funcionais](#2-requisitos-funcionais)
   - [RF1 — Ingestão de múltiplas fontes](#rf1--ingestão-de-múltiplas-fontes)
   - [RF2 — Modelo de evento normalizado](#rf2--modelo-de-evento-normalizado)
   - [RF3 — Consulta por timeline](#rf3--consulta-por-timeline)
   - [RF4 — Consulta semântica](#rf4--consulta-semântica)
   - [RF6 — Relatório de tarefas](#rf6--relatório-de-tarefas)
   - [RF5 — Interface CLI](#rf5--interface-cli)
3. [Requisitos não funcionais](#3-requisitos-não-funcionais)
4. [Critérios de aceite](#4-critérios-de-aceite)
5. [Escopo de entrega](#5-escopo-de-entrega)
6. [Stack](#6-stack)
7. [Limitações conhecidas](#7-limitações-conhecidas)
8. [Fora de escopo](#8-fora-de-escopo)

## 1. Visão geral

CLI que ingere a atividade do usuário de várias fontes (git, browser, arquivos e mensagens do Teams), normaliza tudo num modelo único de evento e permite duas formas de consulta: reconstrução de timeline por data e busca semântica por conteúdo. Todo processamento — banco, embeddings e geração de linguagem — roda localmente, sem enviar dados para serviços externos.

---

## 2. Requisitos funcionais

### RF1 — Ingestão de múltiplas fontes
- **RF1.1** Ingerir histórico de repositórios Git (commits: hash, data, autor, mensagem, arquivos alterados), de todas as branches, tags e remotes. Filtro opcional por autor. Cada commit é marcado como do usuário ou de outra pessoa pelas identidades de `sources.git_identities` (`"auto"` = `git config user.email`/`user.name` do repositório; commits gravados antes são marcados na ingestão seguinte). A `timeline` mostra só os do usuário (`--all-authors` mostra todos), perguntas em primeira pessoa sem pessoa citada deixam de fora os de outros, e o relatório de tarefas ignora commits de outros e conta os do usuário como trabalho seu na tarefa.
- **RF1.2** Ingerir histórico de browser a partir do banco SQLite local (URL, título, timestamp de cada visita). Suporta Firefox (`places.sqlite`) e navegadores Chromium (`History`). O banco é lido a partir de uma cópia, pois o navegador o mantém travado.
- **RF1.3** Ingerir arquivos de diretórios configurados (caminho, data de modificação e conteúdo textual quando aplicável). Arquivos binários ou maiores que `max_file_bytes` entram sem conteúdo; diretórios em `ignored_dir_names` são ignorados. Cada arquivo é um evento com a versão atual (UID = caminho, revisão = data de modificação); cada versão vista fica em `file_modifications` (caminho, data, tamanho), e a timeline mostra todas as edições. Um arquivo que sumiu da pasta ganha `removed_at` na ingestão seguinte (só quando a pasta inteira foi lida sem erro), sai das respostas e continua na timeline; se voltar, a marca é removida.
- **RF1.4** Ingerir mensagens de chat do Teams a partir do IndexedDB do Teams web no Chrome (store `replychains`), sem Graph API e sem rede. Entram apenas mensagens de conversa (`RichText/Html` e `Text`); chamadas, gravações, eventos de sistema, mensagens apagadas e cópias do feed de notificações são ignorados. Cada mensagem registra o tipo de conversa (chat, canal, reunião) e se foi enviada, recebida ou publicada num canal.
- **RF1.5** A ingestão é **incremental**: reexecutar não duplica eventos já ingeridos (deduplicação por identificador estável do evento).
- **RF1.6** Uma mudança de formato do Teams não passa em silêncio: se o IndexedDB tem registros mas nenhum store `replychains`, ou mensagens em que nenhuma tem os campos lidos (`id`, `conversationId`, `messageType`, `content` e o horário de chegada), a ingestão falha apontando para `cade teams-schema`. Mensagens de sistema ou apagadas continuam descartadas sem erro, e um IndexedDB vazio não é erro. Os eventos já gravados estão no formato do cade e não dependem do formato do Teams.

### RF2 — Modelo de evento normalizado
- **RF2.1** Toda fonte é convertida a um formato comum de evento: `timestamp`, `source` (git/browser/file/teams), `content` (texto), `metadata` (dados específicos da fonte) e um identificador único para deduplicação.
- **RF2.2** Cada evento com conteúdo textual tem embeddings gerados localmente para busca semântica, um por pedaço: texto de até 1.200 caracteres é um pedaço só; texto maior é dividido (`internal/chunking`: títulos Markdown, parágrafos, linhas, espaços; ~10% de sobreposição) para caber no contexto de treino do embedder (512 tokens, lido do GGUF). `chunks` guarda as posições no texto, sem duplicá-lo. Eventos sem texto entram apenas na timeline.
- **RF2.3** Os metadados de cada fonte têm um tipo próprio (`event.Commit`, `event.Visit`, `event.File`, `event.Message`), lido e escrito só pelo pacote `event`; o resto do código não usa chaves em texto. O formato gravado continua o mesmo objeto JSON com as mesmas chaves, então eventos já gravados são lidos sem migração. Uma fonte nova acrescenta o seu tipo.

### RF3 — Consulta por timeline
- **RF3.1** Consultar todos os eventos de uma data específica, ordenados cronologicamente. Aceita `AAAA-MM-DD`, `hoje` e `ontem`.
- **RF3.2** Consultar um intervalo de datas.
- **RF3.3** A timeline cruza todas as fontes numa visão unificada, em ordem de hora, com filtro opcional por fonte (`--source`).
- **RF3.4** A timeline indica a fonte de cada evento.

### RF4 — Consulta semântica
- **RF4.1** Fazer perguntas em linguagem natural sobre a atividade.
- **RF4.2** Recuperar os eventos mais relevantes por similaridade semântica (busca vetorial), com filtro opcional por fonte (`--source`) e período (`--from`/`--to`). Sem as flags, o LLM local interpreta a pergunta (saída restrita por gramática GBNF) e extrai apenas os filtros explícitos: período, fonte, pessoas, direção e assunto. Filtros, flags e datas são resolvidos num plano único e determinístico (`queryplan.Resolve`); datas vêm de um parser determinístico. Nomes são comparados por palavra inteira, ignorando maiúsculas, acentos, letras dobradas e y/i ("sillva" → "Leandro Silva"); um nome que não casa com nenhum remetente ou conversa vira filtro de texto em listagens e tarefas (um cliente lido como pessoa) e é ignorado em respostas, que já buscam pela pergunta inteira; uma verificação remove direção ou período que a pergunta não sustenta. `--no-filters` desativa.
- **RF4.3** Um LLM local gera a resposta baseada **somente** nos eventos recuperados. Quando nada relevante é encontrado, o sistema informa isso em vez de responder. A busca é por pedaço e a resposta por evento: cada evento vale pelo seu pedaço mais próximo, e o prompt recebe esse pedaço (até 1.200 caracteres), com a posição ("arquitetura.md, trecho 7 de 20"), e não o começo do texto.
- **RF4.4** A resposta cita as fontes e datas dos eventos usados, cada uma com seu localizador (`repositório@hash`, URL, caminho, link da mensagem no Teams). Números citados que não correspondem a nenhum evento consultado são apontados como trechos sem fonte.
- **RF4.5** Pedidos de enumeração com período ("as mensagens que recebi da Ana ontem") listam todos os eventos que casam com os filtros, direto do banco: a busca por similaridade retornaria só os top-k. Um assunto ("páginas sobre redis") restringe a lista por similaridade.
- **RF4.6** Perguntas com pessoa ou direção ("o que o Marcos me pediu ontem?") restringem os eventos exatamente antes de ordená-los por similaridade, pois nomes e direção não são capturados de forma confiável pelo embedding. No Teams, publicações em canal não contam como recebidas, nem mensagens de grupo que só @mencionam outras pessoas conhecidas (o usuário é reconhecido pelas mensagens que enviou; menção a ele, a um time ou tag mantém a mensagem).
- **RF4.7** Só o assunto é buscado por significado quando há filtros exatos (período, fonte, pessoa, direção): em "commits de ontem sobre autenticação", embutir a pergunta inteira trazia mensagens com a palavra "ontem" antes dos commits. Sem filtros, a pergunta inteira é embutida, pois o assunto isolado fica distante demais de tudo e cairia no corte de distância.
- **RF4.8** `cade ask --json` devolve o plano resolvido e o resultado com a referência (uid, fonte, data, localizador) de cada evento que o sustenta: evidências e quais foram citadas, eventos listados, ou tarefas com PRs e eventos.
- **RF4.9** Mensagens formadas só por cumprimentos, confirmações e palavras de ligação ("ok", "valeu", "bom dia", "pode ser") não entram como evidência de respostas: entre as mensagens de uma pessoa, elas ficavam à frente do pedido real. Listagens continuam mostrando tudo.
- **RF4.10** Perguntas sem filtro só recebem evidência se o evento mais próximo estiver a no máximo `retrieval.max_best_distance` (0,62); um corte só por evento não separava perguntas com e sem resposta. Calibrado na suíte de recuperação; no histórico real a margem é menor, por isso é configurável e o `--verbose` registra a distância quando a pergunta é rejeitada.
- **RF4.11** Nas respostas, repetições da mesma coisa (mesma URL, mesmo caminho de arquivo, mesmo commit ou mensagem, pelo localizador da proveniência) viram uma evidência só, com a contagem e a data mais recente ("12 visitas, última em …"); a busca pede mais vizinhos até ter `top_k` itens distintos. Arquivos removidos da pasta não entram como evidência. Listagens continuam mostrando cada ocorrência.
- **RF4.12** Texto idêntico é embutido uma vez: a ingestão e o `reindex` reaproveitam o vetor de um evento com o mesmo `content_hash`.
- **RF4.13** Busca híbrida (`retrieval.mode = hybrid`, padrão): a busca vetorial e a por palavras (FTS5/BM25 sobre os pedaços, com hash de commit e caminho de arquivo no primeiro pedaço) usam os mesmos filtros e são combinadas por fusão de posições (RRF, k = 60). Uma pergunta com identificador explícito (código `[A-Z]+-\d+`, hash hexadecimal de 7 a 40 caracteres, número de PR) é respondida só com os eventos que o contêm, sem as portas de distância. `vector` e `lexical` usam uma busca só. Perguntas com pessoa continuam só vetoriais.

### RF6 — Relatório de tarefas
- **RF6.1** `cade tasks [DATA [FIM]]` lista as tarefas trabalhadas no período, reconhecidas por links de rastreadores (regex configuráveis em `tasks.task_url_patterns`) em visitas e mensagens.
- **RF6.2** Uma tarefa está concluída quando um PR aberto pelo usuário está ligado a ela (regra do usuário: PR aberto = tarefa finalizada). Abertura detectada localmente: visita à página de criação do PR logo antes da primeira visita ao PR, ou mensagem enviada com o link. Hosts: GitHub, GitLab, Bitbucket, Azure DevOps.
- **RF6.3** Ligação PR ↔ tarefa: exata (mensagem que cita uma única tarefa e o PR, ou título do PR citando o id da tarefa) ou provável (PR aberto até 2h após trabalhar na tarefa), sempre indicada.
- **RF6.4** Perguntas sobre tarefas no `cade ask` ("quais tarefas finalizei ontem?", "what tasks are still in progress?") devolvem o mesmo relatório, com filtro opcional de status (concluídas / em andamento) e período padrão de hoje. Com pessoa ou direção ("tarefas que a Ana me passou ontem"), só entram tarefas com link nas mensagens selecionadas, de qualquer dono; um nome que não é de ninguém filtra pelo texto da tarefa, PRs e mensagens ("tarefas de Solaris" → PR `-> main-solaris`). Uma mensagem citando várias tarefas conta para todas. O tipo "tarefas" e o status só valem se a pergunta os sustentar (menciona tarefa/ticket/demanda; "finalizei", "pendentes"...).
- **RF6.5** Tarefas são classificadas como suas (PR aberto ou mensagem sua citando-a), consultadas (só a página aberta) ou citadas só por outras pessoas (resumidas; `--all` lista).

### RF5 — Interface CLI
- **RF5.1** `cade ingest <fonte|all> [ALVO...]`: ingere uma fonte com alvos explícitos ou os configurados, exibindo progresso durante a execução.
- **RF5.2** `cade timeline [--source F] DATA [DATA_FIM]`.
- **RF5.3** `cade ask [--source F] [--from D] [--to D] PERGUNTA`.
- **RF5.4** Saída legível no terminal, indicando fonte e timestamp de cada resultado.
- **RF5.5** `cade forget <fonte>` remove os eventos de uma fonte para reingestão; `cade init` cria o arquivo de configuração padrão; `cade teams-schema DIR` imprime a estrutura (sem valores) de um IndexedDB, para diagnosticar mudanças de formato do Teams.

---

## 3. Requisitos não funcionais

### RNF1 — Privacidade (crítico)
- **RNF1.1** Todo processamento ocorre localmente. Nenhum dado de atividade é enviado a serviços externos.
- **RNF1.2** A geração de linguagem usa um LLM local via llama.cpp embutido no processo (sem servidor).
- **RNF1.3** A geração de embeddings ocorre localmente, também via llama.cpp.
- **RNF1.4** O banco e seus arquivos auxiliares (`-wal`, `-shm`) são legíveis só pelo dono (`600`, pasta `700`), inclusive bancos criados por versões anteriores, que eram `644`.
- **RNF1.5** Apagar é definitivo no disco: `secure_delete` zera texto apagado ou substituído, e `cade forget` compacta o banco e esvazia o WAL. O `forget file` apaga também o histórico de edições.
- **RNF1.6** `PRIVACY.md` / `PRIVACY.pt-BR.md` documentam o que cada fonte guarda, onde, o que o modelo local vê, os logs, como apagar e os limites (banco sem criptografia; mensagens apagadas no Teams continuam até `forget`).

### RNF2 — Persistência
- **RNF2.1** Eventos e embeddings são armazenados em SQLite com a extensão sqlite-vec. Banco em arquivo local, sem servidor.
- **RNF2.2** O acesso ao banco fica atrás da interface `EventStore`, de modo que o backend possa ser trocado (ex.: PostgreSQL/pgvector) sem alterar ingestão, timeline ou consulta semântica.
- **RNF2.3** Mudanças de esquema são migrações numeradas (`PRAGMA user_version`), aplicadas em ordem ao abrir o banco, cada uma numa transação com a nova versão: uma falha mantém a última versão boa. Migrações que reescrevem dados copiam o banco antes (`VACUUM INTO`, `cade.db.before-vN-<data>`, permissão `600`). Um banco de versão mais nova que o `cade` é recusado. Reimportar não é caminho de migração: o cache do Teams e o histórico do navegador expiram. Versões: 1 = esquema de antes do versionamento; 2 = texto original de cada mensagem do Teams guardado nos metadados (recuperado do conteúdo só quando remontá-lo reproduz o conteúdo exato), para que mudanças no formato do conteúdo sejam refeitas a partir do banco (`event.Message.Content()`); 3 = `content_hash` para reaproveitar o vetor de textos idênticos (dado derivado, sem cópia); 4 = um evento por arquivo, versões em `file_modifications` (com cópia); 5 = vetores por pedaço (`chunks`, `chunk_embeddings`): eventos curtos mantêm o vetor como pedaço único, os longos ficam para `cade reindex` (marcado como pendente), e o banco é compactado no fim; 6 = índice FTS5 `chunks_fts` sobre os pedaços (dado derivado, sem cópia). Uma abertura faz no máximo uma cópia, antes do primeiro passo que reescreve dados.
- **RNF2.4** `cade reindex` recalcula todos os vetores com o modelo de embedding configurado, a partir do texto guardado, em lotes transacionais; se interrompido, a próxima execução continua (mesmo modelo) ou recomeça (outro modelo). O banco registra o modelo dos vetores (`store_settings.embedding_model`); `ingest` e `ask` recusam um modelo diferente — dimensão igual não basta (nomic v1.5 e v2-moe têm 768) — e avisam quando há reindexação pendente. Bancos anteriores adotam o modelo configurado.

### RNF3 — Integridade dos dados (crítico)
- **RNF3.1** A ingestão não perde eventos das fontes suportadas dentro do escopo configurado. Uma falha interrompe a execução em vez de descartar eventos; reexecutar continua de onde parou. Exceção: o Teams é "melhor esforço", pois o IndexedDB contém apenas o que o cliente carregou.
- **RNF3.2** A deduplicação não descarta eventos distintos que coincidam em algum campo:

  | Fonte | Identificador |
  |---|---|
  | git | hash do commit |
  | browser | navegador + URL + instante da visita (µs) |
  | file | caminho + data de modificação (ns) + tamanho |
  | teams | id da conversa + id da mensagem |
- **RNF3.3** Reingerir um evento já gravado só o reprocessa se o conteúdo mudou na fonte: texto e embedding são substituídos numa transação. Quando o evento tem revisão (no Teams, o `version` da mensagem, que muda a cada edição), só uma revisão maior substitui, então a versão editada vence mesmo que outro cache tenha a antiga, e nada alterna entre execuções.

### RNF4 — Extensibilidade
- **RNF4.1** Adicionar uma nova fonte não exige mudança no modelo de evento nem nas consultas — apenas um novo `EventCollector` registrado em `cmd/cade/sources.go`.

### RNF5 — Desempenho
- **RNF5.1** A busca retorna em tempo interativo para o volume de uso pessoal.
- **RNF5.2** Cada modelo é carregado uma única vez por execução.
- **RNF5.3** `make bench` mede latência e memória; `bench/baseline.txt` guarda a referência (RTX 3060, 2026-09-26). Nessa máquina: busca vetorial em 100 mil eventos, 106 ms; ler o histórico inteiro (pergunta com pessoa e sem período), 343 ms; embedding de um evento, ~3,5 ms (~300 eventos/s na ingestão; a primeira medição, sem aquecimento da GPU, deu 27 ms); interpretar a pergunta, 1,3 s; gerar a resposta com 8 evidências, ~0,4 s; ~3,5 KB por evento no banco (o vetor ocupa 3 KB); modelos ocupam ~2,5 GB de GPU e ~1,1 GB de RAM.

### RNF6 — Configuração
- **RNF6.1** Fontes (repositórios, históricos do browser, diretórios, diretórios do Teams), modelos e parâmetros de busca são configuráveis em `~/.config/cade/config.json`, sem hardcode.

### RNF7 — Qualidade da interpretação
- **RNF7.1** Uma suíte de ~150 perguntas representativas (`testdata/queries/plan.json`: período, git, Teams, navegador, arquivos, busca semântica, pessoas, tarefas, empresas lidas como pessoa, ambíguas, PT e EN) fixa o plano esperado de cada uma. `make eval-plan` roda a suíte com o modelo real e mede o acerto por campo com intervalo de Wilson de 95%.
- **RNF7.2** Cada campo tem um piso (`minimum_accuracy`) comparado com o limite inferior do intervalo: mudanças de prompt ou modelo que o derrubem falham o teste, em vez de regredirem em silêncio, e um erro isolado não reprova.
- **RNF7.3** Suíte de recuperação (`testdata/queries/retrieval/`, `make eval-retrieval`): corpus sintético de ~290 eventos com distratores parecidos, visitas repetidas, versões de arquivo, notas longas, commits de outros autores e conversa do dia a dia, ingerido num SQLite real com o embedder real. Os casos se dividem em calibração (só relata onde os limites deveriam ficar) e teste (nunca usado para ajustar, com pisos de recall, MRR e rejeição); mede também a redundância. `make eval-scale` gera a curva por tamanho do corpus (`bench/retrieval-scale.txt`); o baseline antes da próxima versão fica em `bench/retrieval-baseline.txt`.

---

## 4. Critérios de aceite

| ID | Critério | Status |
|---|---|---|
| CA1 | Cada commit de um repositório configurado vira um evento com data, mensagem e arquivos alterados. | Atendido |
| CA2 | Cada visita do histórico do browser vira um evento com URL, título e timestamp. | Atendido |
| CA3 | Cada arquivo de um diretório configurado vira um evento com caminho e data de modificação. | Atendido |
| CA4 | Reexecutar a ingestão não duplica eventos. | Atendido |
| CA5 | A timeline de uma data mostra todos os eventos do dia, de todas as fontes, por hora, com a fonte indicada. | Atendido |
| CA6 | A timeline de um intervalo mostra os eventos em ordem cronológica. | Atendido |
| CA7 | Uma data sem atividade retorna indicação clara de que não há eventos. | Atendido |
| CA8 | Uma pergunta retorna resposta baseada nos eventos recuperados, com fontes e datas citadas. | Atendido |
| CA9 | Uma pergunta sem resposta nos dados retorna "Não encontrei informação" em vez de inventar. | Atendido |
| CA9.1 | A busca vetorial combinada com filtro de fonte e/ou período respeita o filtro. | Atendido |
| CA10 | Nenhuma operação faz requisição de rede com dados de atividade. | Atendido por construção; verificação de tráfego pendente |
| CA11 | Um novo ingestor torna seus eventos consultáveis sem alterar as consultas. | Atendido (Teams foi adicionado assim) |

Notas:
- **CA8** — A qualidade da resposta depende do modelo de geração. O Qwen2.5-3B nem sempre cita os eventos com `[n]`; nesse caso a CLI lista todos os eventos consultados.
- **CA9.1** — O filtro é aplicado dentro da busca KNN do sqlite-vec (colunas de metadata do `vec0`), não após o corte top-k; há teste de regressão para isso. O período vem das flags ou de uma data citada na pergunta ("o que pesquisei semana passada" filtra a semana anterior). Dias da semana ("na segunda") ainda não são reconhecidos.
- **CA10** — O código não usa cliente de rede e o llama.cpp é compilado sem suporte a download. Falta a verificação prática, por exemplo com `strace -f -e trace=network cade ask "..."` ou executando sem rede (`unshare -rn cade ask "..."`).

---

## 5. Escopo de entrega

| Fase | Conteúdo | Status |
|---|---|---|
| 1 | RF2, RF1.1 (git), RF3 (timeline), persistência local | Concluída |
| 2 | RF1.2 (browser), RF1.3 (arquivos), RF4 (busca semântica com LLM local) | Concluída |
| 3 | RF4.4 (citações), filtro por fonte/data combinado com busca vetorial | Concluída |
| 4 | RF1.4 (mensagens do Teams via IndexedDB) | Concluída |

---

## 6. Stack

| Componente | Escolha |
|---|---|
| Linguagem | Go |
| Persistência | SQLite (`mattn/go-sqlite3`) + sqlite-vec |
| Inferência | llama.cpp (tag `b11195`), compilado estático e ligado via cgo; CUDA opcional (`make cuda`) |
| Embeddings | nomic-embed-text-v2-moe Q4_K_M (multilíngue) |
| Geração | Qwen2.5-3B-Instruct Q4_K_M |
| Teams | leitor próprio de LevelDB, IndexedDB do Chromium e serialização V8 |

**Decisões de design:**
- llama.cpp embutido em vez de Ollama ou `llama-server`, para não depender de um processo servidor.
- O nomic-embed-text-v1.5 foi substituído pelo v2-moe por ser centrado em inglês e ordenar mal perguntas em português.
- O Qwen2.5-1.5B foi substituído pelo 3B, que respondia "não encontrei" indevidamente em perguntas com filtro.
- Teams lido do IndexedDB local em vez da Graph API: dispensa login, consentimento do tenant e acesso à rede, ao custo de cobrir apenas o que o cliente armazenou.
- O LevelDB é lido por implementação própria (sem `goleveldb`, que está sem manutenção), mantendo a versão de maior sequência de cada chave, o que é correto independentemente do comparador `idb_cmp1` do Chromium.

---

## 7. Limitações conhecidas

- Arquivos: apenas o início do texto (até `max_file_bytes`) é indexado; não há divisão em trechos.
- Teams: o formato interno pode mudar em atualizações do cliente; mensagens apagadas depois de ingeridas continuam no banco.
- Trocar o modelo de embedding exige um banco novo (a dimensão dos vetores é fixada no primeiro insert).
- As flags de cada comando devem vir antes dos argumentos posicionais.
- Tarefas passadas sem link (só em texto) não são reconhecidas.
- O modelo de 3B às vezes lê empresas e clientes como pessoas; o filtro de texto para nomes desconhecidos compensa em listagens e tarefas.

---

## 8. Fora de escopo

- Interface gráfica ou web.
- Sincronização entre máquinas.
- Fontes que exijam serviços externos.
- Multiusuário.
- Calendário e histórico de chamadas do Teams.
