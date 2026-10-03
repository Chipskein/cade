<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Referência de configuração

`cade init` grava `~/.config/cade/config.json`. Você pode usar o
[`config.example.json`](../config.example.json) como referência completa.

| Campo | Finalidade |
|---|---|
| `database_path` | Caminho do banco SQLite. |
| `ingest.redact` | Mascara segredos reconhecidos no texto (padrão `true`). Globs de arquivos e remoção de parâmetros de URL sempre valem. |
| `ingest.max_image_bytes` | Imagens maiores que isso não são descritas (padrão 20 MiB); ficam só com o nome, como os outros binários. |
| `ingest.max_images_per_run` | Imagens novas descritas por `ingest` (padrão 50, ~20 min numa CPU de 6 núcleos); as outras ficam para as próximas execuções. |
| `ingest.background.threads` | Threads de CPU dos dois modelos nas execuções `ingest start` e `ingest --gentle` (padrão `2`; `0` = núcleos físicos). |
| `ingest.background.gpu_layers` | Camadas na GPU dos dois modelos nessas execuções (padrão `-1` = todas); `0` deixa a GPU livre. |
| `ingest.background.busy_percent` | Parte do tempo em que os modelos podem trabalhar nessas execuções (padrão `50`, de `1` a `100`); eles descansam depois de cada chamada, e é isso que limita a GPU. |
| `ingest.background.max_images_per_run` | Substitui `ingest.max_images_per_run` nessas execuções (padrão `500`). |
| `ingest.retention.max_age_days` | Limite opcional de idade, em dias, por fonte; vazio por padrão. Exemplo: `{ "teams": 365 }`. Aplicado depois de cada ingestão da fonte. |
| `ingest.retention.max_age_days.git` | Defina uma idade positiva em dias para git; `0` desativa a retenção. |
| `ingest.retention.max_age_days.browser` | Defina uma idade positiva em dias para browser; `0` desativa a retenção. |
| `ingest.retention.max_age_days.file` | Defina uma idade positiva em dias para arquivos; `0` desativa a retenção. |
| `ingest.retention.max_age_days.teams` | Defina uma idade positiva em dias para Teams; `0` desativa a retenção. |
| `embedding.model_path` | Caminho do modelo GGUF de embeddings. |
| `embedding.context_tokens` | Limite de contexto/tokens do embedding. |
| `embedding.threads` | Threads de CPU para embedding (`0` = núcleos físicos). |
| `embedding.gpu_layers` | Camadas na GPU para embedding (`-1` = todas). |
| `embedding.query_prefix` | Prefixo usado nos embeddings de consulta. |
| `embedding.document_prefix` | Prefixo usado nos embeddings de documento. |
| `generation.model_path` | Caminho do modelo GGUF de geração. |
| `generation.context_tokens` | Limite de contexto/tokens da geração. |
| `generation.threads` | Threads de CPU para geração (`0` = núcleos físicos). |
| `generation.gpu_layers` | Camadas na GPU para geração (`-1` = todas). |
| `vision.projector_path` | Projetor de visão (GGUF mmproj) do modelo de geração; só o `ingest` carrega, para descrever imagens. |
| `vision.context_tokens` | Contexto da geração ao descrever uma imagem (padrão 2048: imagem de 1024 px, o prompt e a descrição). |
| `retrieval.top_k` | Quantidade de eventos considerados como evidência. |
| `retrieval.max_distance` | Corte de relevância para perguntas sem filtros. |
| `retrieval.max_best_distance` | Qualidade mínima do melhor resultado para responder. |
| `retrieval.max_answer_tokens` | Tamanho máximo da resposta em tokens. |
| `retrieval.mode` | Modo de busca (`hybrid`, `vector` ou `lexical`). |
| `retrieval.max_filtered_events` | Limite para ranqueamento por evento em consultas filtradas. |
| `sources.git_repositories` | Repositórios usados no `ingest git`. |
| `sources.git_authors` | Restringe ingestão a autores selecionados. |
| `sources.git_identities` | Identidades tratadas como "você". |
| `sources.browser_histories` | Arquivos de histórico usados no `ingest browser`. |
| `sources.teams_indexeddb_dirs` | Diretórios IndexedDB do Teams usados no `ingest teams`. |
| `sources.indexeddb_schema_dir` | Pasta dos schemas de IndexedDB, um `<nome>.json` cada, gerados pelo `cade schema-discover` e editáveis à mão (padrão `~/.config/cade/idb-schemas`). |
| `sources.indexeddb_dirs` | Diretórios IndexedDB que cada schema lê, pelo nome do schema, ex.: `{"whatsapp": ["~/.floorp/<perfil>/storage/default/https+++web.whatsapp.com/idb"]}`; o `ingest <nome>` usa esses diretórios. |
| `sources.directories` | Pastas usadas no `ingest file`. |
| `sources.ignored_dir_names` | Nomes de pastas ignoradas na ingestão de arquivos. |
| `sources.ignored_file_globs` | Padrões de nomes de arquivo ignorados; ao configurar, substitui a lista padrão inteira. |
| `sources.max_file_bytes` | Tamanho máximo de arquivo para ingerir texto. |
| `sources.images` | Descreve os arquivos png, jpeg e webp de `sources.directories` com o modelo de visão local (padrão `false`; o `cade init` pergunta). |
| `ui.language` | Idioma da interface (`auto`, `pt`, `en`). |
| `ui.date_order` | Ordem de leitura de datas numéricas. |
| `tasks.task_url_patterns` | Regexes usadas para detectar links de tarefas. |

## Links de tarefas

O `cade tasks` mostra **PR aberto** quando o histórico local registra uma visita à página de criação do PR; sem rede, não dá para saber se ele foi aprovado ou mergeado. Para reconhecer um rastreador específico do projeto, como o Proj4me, adicione um padrão:

```json
"task_url_patterns": ["proj4\\.me/projects/(\\d+)/tasks/(\\d+)"]
```

## Comportamento da ingestão de arquivos

**O que é lido:** texto UTF-8 válido (sem bytes NUL) até `sources.max_file_bytes` (padrão 256 KiB). Arquivos maiores são registrados sem conteúdo. Imagens (png, jpeg, webp) com `sources.images` ligado recebem uma descrição do modelo de visão local no lugar do texto bruto.

**O que é ignorado:**
- Arquivos não-UTF-8 e arquivos com bytes NUL (binários).
- Diretórios cujo nome está em `sources.ignored_dir_names` (padrão: `.git`, `node_modules`, `vendor`, `__pycache__`, `.venv`, `target`) — toda a subárvore é pulada.
- Arquivos que casam com `sources.ignored_file_globs` (padrão: `.env*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`, `credentials*`, `.netrc`, `.npmrc`, `.pypirc`, `.git-credentials`). Definir esse campo substitui a lista padrão inteira.

**Pedaços:** o texto é dividido em pedaços de 1.200 caracteres com sobreposição de 120 caracteres, para que uma frase cortada na borda apareça inteira em um dos pedaços. Cada pedaço recebe seu próprio vetor de embedding.

**Detecção de mudança:** o cade guarda um hash do conteúdo por arquivo. Um arquivo sem alteração é ignorado na próxima execução; um arquivo alterado substitui a entrada anterior. Um arquivo removido da pasta é marcado como apagado e deixa de aparecer nos resultados.

## Como a busca funciona

O `cade ask` executa uma **busca híbrida**: similaridade vetorial (embedding contra pedaços) e palavras-chave (BM25/FTS5) são executadas de forma independente e depois mescladas por **reciprocal rank fusion** — a pontuação de cada evento é `Σ 1/(60 + posição)` nas duas listas, para que o topo de nenhuma delas afogue a outra. O modo pode ser alterado com `retrieval.mode` (`hybrid`, `vector` ou `lexical`).

A busca vetorial percorre todos os pedaços de forma linear (sem índice aproximado). Numa CPU de 6 núcleos, isso custa ~114 ms para 100 mil eventos (sem filtro; veja [`bench/baseline-cpu.txt`](../bench/baseline-cpu.txt)). Um filtro de data ou fonte reduz proporcionalmente. Medições detalhadas em [BENCHMARKS.pt-BR.md](BENCHMARKS.pt-BR.md).

## Espaço em disco

Os vetores são a maior parte do banco. O sqlite-vec os guarda em blocos de 1.024 posições e nunca reaproveita uma posição apagada, então o `cade forget`, os eventos atualizados e as migrações deixam posições vazias que a busca continua lendo. O `cade doctor` avisa quando 20% delas estão vazias; o `cade compact` então reescreve os blocos só com os vetores vivos e diminui o arquivo. Não carrega modelo e mantém os mesmos resultados, mas precisa de espaço livre para uma cópia dos vetores (~3 KB por pedaço) enquanto roda. O `cade reindex` deixa os blocos cheios e compacta o arquivo sozinho. Medições em [BENCHMARKS.pt-BR.md](BENCHMARKS.pt-BR.md#compactação-dos-vetores-65).

## Custo da descrição de imagens

Com `sources.images` ligado, cada imagem custa cerca de 1,8 s numa RTX 3060 e 21 s numa CPU de 6 núcleos: uma pasta com 1.000 capturas leva ~30 min com GPU e ~6 h em CPU, divididos entre execuções por `ingest.max_images_per_run`. Medições em [BENCHMARKS.pt-BR.md](BENCHMARKS.pt-BR.md).

## Fonte Teams *(experimental)*

- Depende do formato interno do IndexedDB que o cliente web do Teams grava num perfil Chromium — o formato pode mudar sem aviso.
- Só enxerga o que o cliente tem em cache. Uma conversa que você nunca abriu não tem mensagens aqui.
- Mensagens apagadas na origem depois da ingestão permanecem no cade até `cade forget`.
- Ingere apenas mensagens de chat — não eventos de calendário nem histórico de chamadas.

Aponte `sources.teams_indexeddb_dirs` para o diretório do IndexedDB do perfil do Teams: um diretório `*.indexeddb.leveldb` no Chromium (ex.: `~/.config/teams-for-linux/Partitions/teams-4-linux/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb`) ou o diretório `idb` da origem do Teams no Firefox e no Floorp (ex.: `~/.floorp/<perfil>/storage/default/https+++teams.microsoft.com/idb`).

## Outros aplicativos por schemas de IndexedDB *(experimental)*

Aplicativos que guardam os dados no IndexedDB do navegador podem ser ingeridos sem código próprio. O modelo local escreve um **schema** uma vez por aplicativo: um arquivo JSON que diz em que store estão as mensagens e de onde vem cada campo. A ingestão lê com o schema e nunca roda o modelo. São lidos perfis Chromium (`*.indexeddb.leveldb`) e perfis do Firefox e do Floorp (`storage/default/<origem>/idb`).

Um schema diz qual armazenamento do navegador ele lê (`records.kind`). O IndexedDB (`indexeddb`) é o único que o cade lê hoje; `local_storage`, `opfs`, `http_cache` e `cache_api` são aceitos num schema e passam a ser lidos à medida que os leitores chegarem. Os comandos descobrem qual armazenamento um diretório guarda pela estrutura dele.

1. `cade schema-discover --name whatsapp <diretório>` lê o diretório, pergunta ao modelo (duas perguntas curtas, sob uma gramática que só aceita caminhos que existem) e salva `whatsapp.json` em `sources.indexeddb_schema_dir`. `--print` só mostra; `--force` substitui um já salvo; `--source` dá a fonte dos eventos quando ela é diferente do nome.
2. Acrescente o diretório em `sources.indexeddb_dirs` (o comando mostra a linha) e rode `cade ingest whatsapp`. Cada schema salvo vira uma fonte com o mesmo nome; um com o nome de uma fonte embutida (`teams`, `git`…) é ignorado.
3. Revise o schema: o rascunho de um modelo pequeno às vezes lê um campo do caminho errado. Na amostra do Teams ele acertou o store, os ids, o remetente e as horas, mas não pôs condição para mensagens de sistema; no WhatsApp Web leu a conversa do id de uma chave de criptografia. Edite o arquivo à mão; ele é conferido ao ser carregado.

**Formato do schema** (`version` 2, `target` `message/1`):

| Chave | Significado |
|---|---|
| `records` | `kind` é o armazenamento (`indexeddb`…); `namespace_prefix` e `container` escolhem os registros (no IndexedDB, o começo do nome do banco e o object store); `each`, quando presente, os itens de cada registro que são mensagens (`$.messageMap.<id>`). |
| `require` | Condições que um item precisa cumprir: `in` / `not_in` (valores do texto em `path`), `kind_not` (`object`, `undefined`…; um valor ausente é `undefined`). |
| `fields` | `message_id`, `conversation_id`, `sent_at` (obrigatórios); `sender`, `sender_id`, `conversation`, `text`, `sent_by_me`, `revision`. |
| `paths` do campo | Lidos em ordem; o primeiro com valor vence. |
| `split` do campo | `separator` e `index`: fica com uma parte de um valor composto (`false_<chat>_<id>`). Um flag também lê uma parte igual a `true`. |
| `transform` do campo | Texto: `trim`, `html_text`. Hora: `unix_ms`, `unix_s`, `iso8601`; um `Date` do JavaScript não precisa de nenhum. |
| `lookup` do campo | Lê `values` (o primeiro com valor) do registro em outro `namespace_prefix` e `container`, do mesmo armazenamento, cujo `match` é igual ao `key_path` do item, ou ao valor de outro campo (`key_field`, um nível). |
| `default`, `required` do campo | O valor quando nada mais tem um; `required` descarta o item quando o campo fica vazio. |

Schemas salvos como `version` 1 (de antes do `kind`) continuam funcionando sem edição: são lidos como `indexeddb`, com `database_prefix` e `store` no lugar de `namespace_prefix` e `container`, e uma substituição os grava como versão 2.

Os caminhos usam a notação que o `cade teams-schema` mostra: `$` é o registro ou o item, `.chave`, `[]` itens de lista, `{}` valores de Map, `<>` membros de Set, e `.<id>` / `.<text>` para chaves que são dados (casam com toda chave assim). O UID de uma mensagem é a fonte, o id da conversa e o id da mensagem, então a mesma mensagem lida duas vezes continua um evento só.

**Quando o aplicativo muda.** Aplicativos mudam o jeito de guardar os dados sem aviso. Um schema feito pelo `schema-discover` guarda uma impressão digital: os caminhos que ele lê com os tipos, sem valores.

- `cade schema-check [NOME…]` compara cada schema com os diretórios dele agora e lista os caminhos que sumiram ou mudaram de tipo. Ele sai com erro enquanto houver mudança sem resolver, então pode rodar num timer; o `ingest` só lê e aponta para ele quando nada mais mapeia.
- `cade schema-check --update` regenera o que mudou. O modelo também lê o schema atual e a mudança. O schema novo só substitui o atual se mapear pelo menos tantas mensagens e mantiver o UID de pelo menos 90% delas; o atual vai para `history/<nome>.<revisão>.json`. Senão, ele é salvo como `<nome>.candidate.json` para revisão, e a ingestão nunca o lê.
- `--rekey` aceita também um schema que muda a identidade das mensagens (o aplicativo trocou o campo de id): as mensagens já indexadas ganham o UID novo, depois de uma cópia `cade.db.before-rekey-<data>`. Uma mensagem esquecida continua esquecida.
- `cade schema-discover --rollback --name NOME` volta à revisão anterior.

`idb-discover` e `idb-check`, os nomes de quando os schemas só liam IndexedDB, continuam rodando os mesmos comandos.

Uma troca de schema nunca apaga eventos já indexados. Uma revisão nova que lê uma mensagem de outro jeito a substitui na próxima ingestão, se o aplicativo ainda a tiver; uma mensagem que já saiu do cache do aplicativo fica com o que a revisão antiga leu.

**Limites.** O WhatsApp Web cifra o corpo das mensagens no IndexedDB, então as mensagens dele são indexadas só por metadados (quem, qual conversa, quando, se foi você que enviou). Os filtros de mensagem por pessoa e direção do `ask` por enquanto só funcionam para o Teams.

## Ingestão em segundo plano

Uma ingestão longa (a primeira, uma pasta com milhares de imagens) pode rodar por horas sem prender um terminal nem a máquina inteira:

```sh
cade ingest start all   # sai do terminal; continua depois de fechá-lo
cade ingest status      # etapa, progresso, ETA, caminho do log
cade ingest pause       # congela: sem uso de CPU nem GPU, modelos mantidos na memória
cade ingest resume      # continua uma pausada, ou roda de novo a última não concluída
cade ingest stop        # encerra e libera a memória; o que já foi gravado fica
```

- **Progresso:** a ingestão em andamento grava a etapa e a linha de progresso em `~/.local/state/cade/ingest-state.json` (ou em `$XDG_STATE_HOME`) cerca de uma vez por segundo; `status` lê de qualquer terminal, e o `cade doctor` mostra quando a última rodou e como terminou. Execuções em segundo plano escrevem a saída em `ingest.log`, na mesma pasta.
- **ETA:** a descrição de imagens e as fontes `file` e `git` mostram um ETA, pelo tempo dos itens recentes; `browser` e `teams` não conseguem contar os eventos antes e mostram a taxa.
- **Pausar ou parar:** `pause` congela o processo (SIGSTOP): ele não usa CPU nem GPU, mas os modelos ficam na memória (~2,5 GB de VRAM no padrão), e `resume` o continua na hora. `stop` o encerra (SIGTERM, como o Ctrl-C) e libera a memória; `resume` então roda de novo, pulando eventos gravados e imagens descritas sem usar os modelos, e continua de onde parou.
- **Uma por vez:** enquanto uma ingestão está rodando ou pausada, iniciar outra (`cade ingest …` ou `start`) é recusado com o pid dela. Uma interrompida ou que falhou não bloqueia novas execuções, para que um timer nunca pare de ingerir por causa de uma falha antiga.

### Como os limites funcionam

`ingest start` e `ingest --gentle` aplicam `ingest.background`, em quatro camadas:

| Camada | Campo | O que limita | O que não limita |
|---|---|---|---|
| Threads | `threads` (2) | núcleos de CPU que o llama.cpp usa; os outros programas ficam com o resto | o trabalho na GPU |
| Camadas na GPU | `gpu_layers` (-1) | quanto de cada modelo roda na GPU: `0` deixa a GPU livre e roda na CPU | a carga da GPU enquanto trabalha |
| Descanso entre chamadas | `busy_percent` (50) | a parte do tempo em que os modelos trabalham: depois de cada embedding ou imagem, a execução espera em proporção ao tempo do trabalho (com 50, 2 s de trabalho, 2 s de descanso), então a carga média de CPU/GPU, o consumo e o calor acompanham essa parte | o pico de consumo durante cada trabalho |
| Prioridade | nenhum | CPU (nice 19) e disco (classe ociosa): qualquer outro programa vem antes | a GPU, que não tem prioridade entre processos |

O descanso é a única alavanca que o cade tem sobre a GPU. Com imagens, cada descrição é ~1,7 s de GPU numa RTX 3060, então com 50 % a GPU alterna ~1,7 s em potência máxima e ~1,7 s parada; o consumo médio cai mais ou menos à metade e a temperatura se estabiliza mais baixa, porque o cooler tem o tempo parado para compensar. Os embeddings levam milissegundos cada, curtos demais para esquentar.

**Um limite rígido de potência ou temperatura** é configurado na própria GPU, não pelo cade: precisa de root e vale para todos os programas que usam a GPU.

```sh
nvidia-smi -q -d POWER            # limite de potência padrão e mínimo
sudo nvidia-smi -pl 110           # limita a 110 W até reiniciar (a RTX 3060 vem com 170 W)
sudo nvidia-smi -lgc 300,1500     # ou limita o clock do núcleo (MHz)
sudo nvidia-smi -rgc              # desfaz o limite de clock
```

Limitar a potência custa pouca velocidade: perto do topo da faixa, a GPU perde muito menos desempenho que consumo. Junto com o `busy_percent`, isso mantém baixos tanto o pico quanto a média.

## Ingestão agendada

> **O Chrome apaga visitas com mais de ~90 dias e o cliente do Teams só guarda em cache o que você abriu — se você pular uma execução, essa janela fecha para sempre.**

`~/.config/systemd/user/cade-ingest.service`:

```ini
[Unit]
Description=cade ingest

[Service]
Type=oneshot
ExecStart=%h/.local/bin/cade ingest --gentle all
```

`~/.config/systemd/user/cade-ingest.timer`:

```ini
[Unit]
Description=Executa cade ingest diariamente

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

```sh
systemctl --user enable --now cade-ingest.timer
```

Ou com crontab:

```cron
0 8 * * * ~/.local/bin/cade ingest --gentle all
```

`--gentle` evita que uma execução agendada deixe lento o que você estiver fazendo na hora; tire-o para ingerir na velocidade máxima. Não use `ingest start` num timer: ele sai do terminal na hora, e o systemd tomaria a execução por concluída.
