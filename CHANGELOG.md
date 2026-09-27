# Changelog

**English** · [Português](CHANGELOG.pt-BR.md)

What changed in each version, the schema migrations, and what each migration rewrites. What is left for the release is listed in [docs/ROADMAP.md](docs/ROADMAP.md). The charts are in [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

## Unreleased (first version)

### Upgrading an existing database

- **When:** migrations run by themselves the first time any command opens the database.
- **Backup:** before the first migration that rewrites data, cade writes one copy, `cade.db.before-vN-<date>`, with permission `600`. You can delete it once you have checked that everything works.
  - On a real copy (108 thousand events, schema 2 → 6), migrating took ~50 s, and the database went from 516 MB to 450 MB.
- **Reindex:** after migration 5, run `cade reindex` once. Long texts have no vectors until it finishes, and `ask` warns about it.
  - On the same copy, it took ~80 s on an RTX 3060.
- **No re-import:** reimporting would lose data, because the Teams cache expires and Chrome keeps ~90 days of history.

### Migrations

| Version | What it does | Backup | Rewrites data |
|---|---|---|---|
| 1 | `events` and `store_settings` tables | — | — |
| 2 | keeps the Teams message text in the metadata | yes | Teams events |
| 3 | `content_hash`, to reuse the vector of identical text | no | adds a column |
| 4 | one event per file; earlier versions become dates and sizes in `file_modifications` | yes | file events (older versions are collapsed, UIDs rewritten) |
| 5 | vectors per chunk (`chunks`, `chunk_embeddings`) instead of per event; the database is compacted afterwards | yes | vectors (long events wait for `cade reindex`) |
| 6 | keyword index over chunks (`chunks_fts`, FTS5) | no | fills the index from existing chunks |
| 7 | people index (`event_people`) and message direction (`events.direction`) | no | fills both from existing events |

The binary must be built with the `sqlite_fts5` tag; `make` does this. Without it, opening the database fails with a clear message.

### Person filters in SQL (phase 6)

- **Problem:** a question with a person and no period ("o que a Ana me mandou?") loaded every event, with its content, and filtered in Go. Memory grew with the database.
- **People index:** the `event_people` table keeps, per event, the sender (or the commit author), the conversation and the @mentioned first names, normalized the way names are compared (whole words, no accents, doubled letters and y/i merged). The `events.direction` column keeps whether a message was sent, received or posted to a channel. Both are written at ingestion, and migration 7 fills them for existing events, without re-ingesting. On a real copy (108k events) it took ~1.4 s and the database grew by 7 MB.
- **Filter in SQL:** period, source, direction and people become one condition over the index. Each name is resolved (full name, then first name) with a count that stops at the first event, and only the matching events are read. The "received" rule (a group message that only mentions other known people does not count) runs in the same SQL.
- **Limit:** above `retrieval.max_filtered_events` (1000) matching events, as in "mensagens que recebi" with no period, the search goes to the vector index for the period and source, with `k` growing up to 4096, and keeps the neighbours that match. Ranking an event reads its vectors (~0.19 ms), so 1000 stay under 0.2 s.
- **Same results:** a test compares the SQL filter with the in-memory filter over 120 combinations (4 scopes × 3 directions × 10 sets of names, with mentions, channels, commits and spelling variants). The retrieval suite gives identical results case by case (recall 1.00, MRR 0.89, rejection 1.00).
- **Measured** (`BenchmarkPersonFilter`, a person with no period, 100k synthetic events):

  | | before | after |
  |---|---|---|
  | time | 471 ms | 8 ms |
  | memory allocated | 187 MB | 0.7 MB |

  At 1k and 10k events, before allocated 1.5 MB and 17 MB; after, 33 KB and 69 KB. After filtering, memory follows the person's events, not the database, and the limit caps it. "Mensagens que recebi" with no period (`BenchmarkDirectionFilter`) allocates ~190 KB at every size.
- **Privacy:** `forget` deletes the `event_people` rows with the events (privacy test).

### Continuous integration (phase 10)

- **On every push and pull request** (`.github/workflows/ci.yml`): `gofmt`, `go vet`, `golangci-lint` and the tests with coverage, all with the `sqlite_fts5` tag. `make check` runs the same locally.
  - The llama.cpp build is cached by `LLAMA_TAG`. It is built with the new `LLAMA_NATIVE=OFF` (AVX2, FMA, F16C), because a library tuned to one runner's CPU can crash on another. Local builds keep `ON`.
- **Model suites off the push path** (`.github/workflows/eval.yml`): `make eval` on CPU, by hand or weekly, with the models and the corpus embeddings cached. The report is uploaded as the `eval-report` artifact. `EVAL_TIMEOUT` (default `1h`) lifts Go's 10-minute test limit, which a CPU runner exceeds.
- **Lint:** `errcheck`, `staticcheck`, `unused` and `ineffassign`, version pinned in the Makefile. What they found was fixed rather than silenced:
  - 75 test lines ignored errors from setup steps (writing fixtures, saving events, running the pipeline), so a broken setup could pass silently or fail in a later, misleading assertion. They now stop the test (`internal/testcheck`).
  - two tests ran a call that fails on purpose without checking that it failed; they now assert the error.
  - the exclusions are golangci's standard set (`Close`, terminal prints, removing temporary files) plus `tx.Rollback` after `Commit`, each with its reason in `.golangci.yml`.
- **Coverage:** 82.1% of statements. The total goes to the run summary and to a README badge, served from a `coverage.json` on the `badges` branch, with no external service.

### Faster `ask` (phase 5)

- **Rules before the model:** a question made only of a period, a source and generic words ("liste os commits de ontem", "o que fiz hoje?", "which tasks did I finish today?") is read without the model. Any other word (a name, a topic, a number) sends it to the model, so the rules never guess.
  - They read 67 of the 153 plan-suite questions, all correctly. The suite went from 129 to 131 fully correct questions, because the model sometimes invented a topic on those.
  - A listing or task report read by the rules loads no model at all; the generator is loaded only when something needs it.
- **Saved prompt state:** the planner's fixed instructions and examples (~2 thousand tokens) used to be decoded again by every `cade ask`. Their state is now saved once in `~/.cache/cade/prompt-state/` (~55 MB, owner-only) and loaded by later runs.
  - The key covers the llama.cpp version and commit, CPU or CUDA build, the model file (path, size, modification time), the context size, the offloaded layers and the prompt tokens. Any change writes a new file and deletes the old one.
  - It holds no question and nothing from the database. Deleting it is safe; it is rebuilt on the next question.
- **Measured** (`BenchmarkColdAsk`, up to the first answer token, both models loaded, page cache warm):

  | | model, whole prompt | model, saved state | rules |
  |---|---|---|---|
  | CPU (Ryzen 5 5500) | 41.5 s | 26.0 s | 21.5 s |
  | GPU (RTX 3060) | 2.93 s | 2.48 s | 1.59 s |

  With the page cache cold (models evicted, as after a reboot), CPU goes 45.2 → 31.2 → 26.6 s and GPU 8.8 → 8.5 → 7.7 s. What is left on CPU is mostly the model reading the evidence before answering.
- **Benchmarks:** `make bench` also times a whole `ask`, and `make bench GO_TAGS=` saves the CPU-only baseline in `bench/baseline-cpu.txt`.

### Git authorship (phase 4)

- **Identities:** each commit is marked `mine` or `other` by `sources.git_identities`.
  - The default, `["auto"]`, reads `git config user.email` and `user.name` in each repository.
  - An unmarked commit counts as yours.
  - Stored commits are re-marked on every `ingest git`, with no migration.
- **Where the mark counts:**
  - `timeline` hides other people's commits; `--all-authors` shows them.
  - First-person questions that name no person ("what did I do…", "o que eu fiz…") leave out other people's commits.
  - Task reports ignore other people's commits, and one of your commits that cites a task makes it yours.
- **Effect:** on a real history, 1,037 of 61,645 commits were the user's. Until now, all of them counted as the user's work.

### Hybrid search (phase 3)

- **Keywords:** FTS5 keyword search (accents ignored) now runs alongside vector search, and the two rankings are fused by reciprocal rank fusion.
- **Identifiers** (task or error codes like `PROJ-481`, commit hashes, PR numbers) act as a strong filter.
- **Modes:** `retrieval.mode` chooses `hybrid` (the default), `vector` or `lexical`.
- **Results:**

  | | recall | MRR |
  |---|---|---|
  | test set, before | 0.87 | 0.81 |
  | test set, after | 1.00 | 0.88 |
  | 10 thousand events, before | 0.80 | 0.80 |
  | 10 thousand events, after | 0.93 | 0.82 |

  Rejection stayed at 1.00.

### Chunks (phase 2)

- **Split:** texts over 1,200 characters are split by Markdown headings, paragraphs, lines and spaces, with a 120-character overlap. Each chunk gets its own vector.
- **In answers:** the prompt receives the chunk that matched ("chunk i of n"), not the start of the file. `--json` shows `chunk`, `chunks`, `excerpt_start` and `excerpt_end`.
- **Results:**

  | | recall | MRR |
  |---|---|---|
  | test set, before | 0.80 | 0.74 |
  | test set, after | 0.87 | 0.81 |

  The gain comes from a long note with the answer near its end.

### Deduplication (phase 1)

- **Repeated visits** to a page, or matches from the same file, show as one item, with the count and the latest date. Timelines still show each visit.
- **Vectors:** identical text is embedded once. On a real history, 89% of browser visits repeat a text that is already stored.
- **Files:**
  - one event per file, with the edit dates in `file_modifications`;
  - files removed from their folder drop out of answers;
  - ten versions of a 200 KB file take the space of one.
- **Results:** redundancy on the test set went from 0.17 to 0.00.

### Fixed

- **Embedder context (phase 0.5):** the embedder ran with a 2,048-token context, but the model was trained on 512.
  - It now uses the smaller of the two, and `--verbose` shows the effective context and every truncation.
  - Vectors of long texts only improve after `cade reindex`.
- **Migrations:** opening the database with several pending migrations made one copy per migration; it now makes one.
- **`forget`:** it now erases file history and the keyword index along with events.

### Evaluation (phase 0)

- **Retrieval suite:**
  - a 291-event corpus;
  - separate calibration (25 questions) and test (24 questions) sets; thresholds come only from the calibration set;
  - a scale curve with synthetic distractors (`make eval-scale`);
  - metrics: recall, MRR, rejection and redundancy.
- **Plan suite:** 153 questions, reported with 95% Wilson intervals. Floors are checked against the lower bound.
- **Anonymization:** `testdata/README.md` explains how to turn a real question into a test case.
- **Benchmarks:** models warm up before measuring.

### Earlier in this version

- **Questions:** `cade ask` plans each question (mode, source, period, people, direction, topic, task status). Filters run in SQL, and only the topic is matched by meaning.
  - Unscoped questions whose closest event is far away are rejected instead of answered.
  - Answers cite their sources.
- **Tasks:** `cade tasks` shows the period's tasks, their pull requests and whether each one is yours.
- **Teams:**
  - message text is kept (migration 2);
  - edited messages replace the old version;
  - ingestion fails clearly on an unrecognized cache format, and `teams-schema` helps diagnose it.
- **Reindex:** `cade reindex` recomputes vectors, and a changed embedding model is detected.
- **Privacy:**
  - the database file is readable only by you, and its directory only by you;
  - replaced and forgotten text is zeroed on disk;
  - see [PRIVACY.md](PRIVACY.md).
- **Help:** `cade help` and `-h` follow the locale (English or Portuguese).
- **License:** GPLv2.
