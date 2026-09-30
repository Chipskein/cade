<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Configuration reference

`cade init` writes `~/.config/cade/config.json`. You can use
[`config.example.json`](../config.example.json) as a full reference.

| Field | Purpose |
|---|---|
| `database_path` | Path to the SQLite database. |
| `ingest.redact` | Mask recognized secrets in event text (default `true`). File globs and URL parameter removal always apply. |
| `ingest.max_image_bytes` | Images larger than this are not described (default 20 MiB); they keep their name only, like other binaries. |
| `ingest.max_images_per_run` | New images described per `ingest` (default 50, ~20 min on a 6-core CPU); the rest wait for the next runs. |
| `ingest.background.threads` | CPU threads for both models in `ingest start` and `ingest --gentle` runs (default `2`; `0` = physical cores). |
| `ingest.background.gpu_layers` | GPU layers for both models in those runs (default `-1` = all); `0` keeps the GPU free. |
| `ingest.background.busy_percent` | Share of the time the models may work in those runs (default `50`, from `1` to `100`); they rest after each call, which is what limits the GPU. |
| `ingest.background.max_images_per_run` | Replaces `ingest.max_images_per_run` in those runs (default `500`). |
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

## Task links

`cade tasks` reports **PR opened** when local history shows a visit to the PR creation page; offline, it cannot tell whether the PR was approved or merged. To recognize a project-specific tracker such as Proj4me, add a pattern:

```json
"task_url_patterns": ["proj4\\.me/projects/(\\d+)/tasks/(\\d+)"]
```

## File ingestion behavior

**What is read:** valid UTF-8 text (no NUL bytes) up to `sources.max_file_bytes` (default 256 KiB). Larger files are still recorded without content. Images (png, jpeg, webp) with `sources.images` on get a description from the local vision model in place of raw text.

**What is ignored:**
- Non-UTF-8 files and files containing NUL bytes (binaries).
- Directories whose name is in `sources.ignored_dir_names` (default: `.git`, `node_modules`, `vendor`, `__pycache__`, `.venv`, `target`) — the whole subtree is skipped.
- Files matching `sources.ignored_file_globs` (default: `.env*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`, `credentials*`, `.netrc`, `.npmrc`, `.pypirc`, `.git-credentials`). Setting this field replaces the entire default list.

**Chunks:** text is split into 1,200-character pieces with 120-character overlap so a sentence cut at a boundary is whole in one of them. Each chunk gets its own embedding vector.

**Change detection:** cade stores a content hash per file. An unchanged file is skipped on the next run; a changed file replaces the previous entry. A file removed from the directory is marked as deleted and no longer appears in results.

## How search works

`cade ask` runs a **hybrid search**: vector similarity (embedding against chunks) and keyword (BM25/FTS5) are run independently, then merged by **reciprocal rank fusion** — each event's score is `Σ 1/(60 + rank)` across both lists, so the top of neither drowns the other. The mode can be changed with `retrieval.mode` (`hybrid`, `vector`, or `lexical`).

Vector search scans all chunks linearly (no approximate index). On a 6-core CPU this costs ~114 ms at 100 k events (unfiltered; see [`bench/baseline-cpu.txt`](../bench/baseline-cpu.txt)). A date or source filter reduces it proportionally. Detailed measurements in [BENCHMARKS.md](BENCHMARKS.md).

## Image description cost

With `sources.images` on, each image costs about 1.8 s on an RTX 3060 and 21 s on a 6-core CPU: a folder of 1,000 screenshots takes ~30 min on a GPU and ~6 h on a CPU, spread over runs by `ingest.max_images_per_run`. Measurements in [BENCHMARKS.md](BENCHMARKS.md).

## Teams source *(experimental)*

- Depends on the internal IndexedDB format the Teams web client writes to a Chromium profile — the format can change without notice.
- Only sees what the client has cached. A conversation you never opened has no messages here.
- Messages deleted at the source after ingestion remain in cade until `cade forget`.
- Ingests only chat messages — not calendar events or call history.

Point `sources.teams_indexeddb_dirs` at the IndexedDB directory of your Chromium-based Teams profile (e.g. `~/.config/teams-for-linux/Partitions/teams-4-linux/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb`).

## Background ingestion

A long ingestion (a first run, a folder of thousands of images) can run for hours without holding a terminal or the whole machine:

```sh
cade ingest start all   # detaches; keeps going after the terminal closes
cade ingest status      # stage, progress, ETA, log path
cade ingest pause       # freezes it: no CPU or GPU work, models kept in memory
cade ingest resume      # continues a paused run, or runs the last unfinished one again
cade ingest stop        # ends it and frees the memory; what it stored so far stays
```

- **Progress:** the running ingestion records its stage and progress line in `~/.local/state/cade/ingest-state.json` (or under `$XDG_STATE_HOME`) about once a second; `status` reads it from any terminal, and `cade doctor` shows when the last one ran and how it ended. Background runs write their output to `ingest.log` in the same folder.
- **ETA:** image descriptions and the `file` and `git` sources show an ETA, from how long the recent items took; `browser` and `teams` cannot count their events up front and show the rate instead.
- **Pause or stop:** `pause` freezes the process (SIGSTOP): it uses no CPU or GPU, but its models stay in memory (~2.5 GB of VRAM with the defaults), and `resume` continues it at once. `stop` ends it (SIGTERM, like Ctrl-C) and frees the memory; `resume` then runs it again, skipping stored events and described images without touching the models, so it picks up where it was.
- **One at a time:** while an ingestion is running or paused, starting another one (`cade ingest …` or `start`) is refused with its pid. An interrupted or failed one does not block new runs, so a timer never stops ingesting because of an old failure.

### How the limits work

`ingest start` and `ingest --gentle` apply `ingest.background`, in four layers:

| Layer | Setting | What it limits | What it does not |
|---|---|---|---|
| Threads | `threads` (2) | CPU cores llama.cpp uses; other programs keep the rest | GPU work |
| GPU layers | `gpu_layers` (-1) | how much of each model runs on the GPU: `0` keeps the GPU free and runs on the CPU | the GPU's load while it works |
| Rest between calls | `busy_percent` (50) | the share of time the models work: after each embedding or image, the run waits in proportion to how long the work took (at 50, 2 s of work, 2 s of rest), so the average CPU/GPU load, power and heat follow that share | the peak power during each piece of work |
| Priority | none | CPU (nice 19) and disk (idle class): any other program comes first | the GPU, which has no priority between processes |

The rest is the only lever cade has on the GPU. With images, each description is ~1.7 s of GPU work on an RTX 3060, so at 50 % the GPU alternates ~1.7 s at full power and ~1.7 s idle; the average power roughly halves and the temperature settles lower, since the cooler has the idle time to catch up. Embeddings are milliseconds each, so their bursts are too short to heat anything.

**A hard power or temperature cap** is set on the GPU itself, not by cade: it needs root and applies to every program using the GPU.

```sh
nvidia-smi -q -d POWER            # default and minimum power limit
sudo nvidia-smi -pl 110           # cap at 110 W until reboot (an RTX 3060 defaults to 170 W)
sudo nvidia-smi -lgc 300,1500     # or cap the core clock (MHz)
sudo nvidia-smi -rgc              # undo the clock cap
```

A power cap costs little speed: GPUs lose much less performance than power near the top of their range. Combined with `busy_percent`, it keeps both the peak and the average down.

## Scheduled ingestion

> **Chrome purges visits older than ~90 days and the Teams client only caches what you have opened — if you skip a run, that window closes for good.**

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
Description=Run cade ingest daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

```sh
systemctl --user enable --now cade-ingest.timer
```

Or with crontab:

```cron
0 8 * * * ~/.local/bin/cade ingest --gentle all
```

`--gentle` keeps a scheduled run from slowing down whatever you are doing when it fires; drop it to ingest at full speed. Do not use `ingest start` in a timer: it detaches at once, and systemd would take the run as finished.
