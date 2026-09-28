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
