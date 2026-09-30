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
