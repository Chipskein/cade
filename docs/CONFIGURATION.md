# Configuration reference

`cade init` writes `~/.config/cade/config.json`. You can use
[`config.example.json`](../config.example.json) as a full reference.

| Field | Purpose |
|---|---|
| `database_path` | Path to the SQLite database. |
| `ingest.redact` | Mask recognized secrets in event text (default `true`). File globs and URL parameter removal always apply. |
| `ingest.max_image_bytes` | Images larger than this are not described (default 20 MiB); they keep their name only, like other binaries. |
| `ingest.max_images_per_run` | New images described per `ingest` (default 50, ~20 min on a 6-core CPU); the rest wait for the next runs. |
| `ingest.retention.max_age_days` | Optional per-source age limit in days; empty by default. Example: `{ "teams": 365 }`. Applied after each source ingestion. |
| `ingest.retention.max_age_days.git` | Set a positive age in days for git; `0` disables retention. |
| `ingest.retention.max_age_days.browser` | Set a positive age in days for browser; `0` disables retention. |
| `ingest.retention.max_age_days.file` | Set a positive age in days for file; `0` disables retention. |
| `ingest.retention.max_age_days.teams` | Set a positive age in days for Teams; `0` disables retention. |
| `embedding.model_path` | GGUF model path for embeddings. |
| `embedding.context_tokens` | Embedding context/token limit. |
| `embedding.threads` | CPU threads for embeddings (`0` = physical cores). |
| `embedding.gpu_layers` | GPU layers for embeddings (`-1` = all). |
| `embedding.query_prefix` | Prefix used for query embeddings. |
| `embedding.document_prefix` | Prefix used for document embeddings. |
| `generation.model_path` | GGUF model path for generation. |
| `generation.context_tokens` | Generation context/token limit. |
| `generation.threads` | CPU threads for generation (`0` = physical cores). |
| `generation.gpu_layers` | GPU layers for generation (`-1` = all). |
| `vision.projector_path` | Vision projector (mmproj GGUF) of the generation model; loaded only by `ingest` to describe images. |
| `vision.context_tokens` | Generation context while describing an image (default 2048: a 1024 px image, the prompt and the description). |
| `retrieval.top_k` | Number of events considered as evidence. |
| `retrieval.max_distance` | Relevance cutoff for unfiltered questions. |
| `retrieval.max_best_distance` | Required best hit quality for answers. |
| `retrieval.max_answer_tokens` | Maximum answer size in tokens. |
| `retrieval.mode` | Retrieval mode (`hybrid`, `vector`, or `lexical`). |
| `retrieval.max_filtered_events` | Limit for per-event ranking in filtered queries. |
| `sources.git_repositories` | Repositories used by `ingest git`. |
| `sources.git_authors` | Restrict ingestion to selected authors. |
| `sources.git_identities` | Identities treated as "you". |
| `sources.browser_histories` | Browser history files used by `ingest browser`. |
| `sources.teams_indexeddb_dirs` | Teams IndexedDB directories used by `ingest teams`. |
| `sources.directories` | Folders used by `ingest file`. |
| `sources.ignored_dir_names` | Folder names ignored by file ingestion. |
| `sources.ignored_file_globs` | File name patterns skipped by file ingestion; overrides the full default list when set. |
| `sources.max_file_bytes` | Max file size to ingest text from. |
| `sources.images` | Describe png, jpeg and webp files of `sources.directories` with the local vision model (default `false`; `cade init` asks). |
| `ui.language` | Interface language (`auto`, `pt`, `en`). |
| `ui.date_order` | Date parsing order for numeric dates. |
| `tasks.task_url_patterns` | Regexes used to detect task links. |
