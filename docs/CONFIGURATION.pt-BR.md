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

A busca vetorial percorre todos os pedaços de forma linear (sem índice aproximado). Numa CPU de 6 núcleos, isso custa ~111 ms para 100 mil eventos (sem filtro; veja [`bench/baseline-cpu.txt`](../bench/baseline-cpu.txt)). Um filtro de data ou fonte reduz proporcionalmente. Medições detalhadas em [BENCHMARKS.md](BENCHMARKS.md).

## Custo da descrição de imagens

Com `sources.images` ligado, cada imagem custa cerca de 1,7 s numa RTX 3060 e 22 s numa CPU de 6 núcleos: uma pasta com 1.000 capturas leva ~30 min com GPU e ~6 h em CPU, divididos entre execuções por `ingest.max_images_per_run`. Medições em [BENCHMARKS.md](BENCHMARKS.md).

## Fonte Teams *(experimental)*

- Depende do formato interno do IndexedDB que o cliente web do Teams grava num perfil Chromium — o formato pode mudar sem aviso.
- Só enxerga o que o cliente tem em cache. Uma conversa que você nunca abriu não tem mensagens aqui.
- Mensagens apagadas na origem depois da ingestão permanecem no cade até `cade forget`.
- Ingere apenas mensagens de chat — não eventos de calendário nem histórico de chamadas.

Aponte `sources.teams_indexeddb_dirs` para o diretório do IndexedDB do perfil Chromium do Teams (ex.: `~/.config/teams-for-linux/Partitions/teams-4-linux/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb`).

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
