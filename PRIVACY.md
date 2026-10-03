<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Privacy

**English** · [Português](PRIVACY.pt-BR.md)

cade reads your commits, browser history, files and Teams messages, so it holds some of the most personal data on your machine. This page lists what it stores, where, what the local model sees, and how to delete it.

## In short

- **Nothing leaves the machine at run time.** The binary has no network code: Go's `net` and `net/http` are not linked, and llama.cpp is built without its downloader. There is no telemetry.
- **Network is used only at build time:** `go tool mage llama` clones llama.cpp from GitHub and `go tool mage models` downloads the two models and the generation model's vision projector from Hugging Face.
- **`cade init` and `cade doctor` look at names, not content:** init lists profile directories and checks which of them hold a `History`, `places.sqlite` or Teams IndexedDB, and looks for `.git` under the directory you name; doctor reads the first bytes of the models and histories (the `GGUF` and `SQLite format 3` signatures) and opens the database read-only.
- **Everything is in one SQLite file**, readable only by your user account (mode `600`, directory `700`).
- **The database is not encrypted.** Anyone logged in as you, or with your disk, can read it. Use disk encryption.

## Where data lives

| Path | Contents | Mode |
|---|---|---|
| `~/.local/share/cade/cade.db` (+ `-wal`, `-shm`) | events, their text, their chunks (offsets into the text, one set per distinct text), embeddings, a keyword index of the words of each chunk, a keyword index of commit hashes and file paths, file edit history | `600` |
| `~/.local/share/cade/cade.db.before-v*` | copy saved before a schema migration that rewrites data; same contents as the database | `600` |
| `~/.config/cade/config.json` | your commit identities if you list them (`git_identities`), and which repositories, histories, directories, Teams profiles and IndexedDB directories to read | `600` |
| `~/.config/cade/idb-schemas/` | one schema per app: database and store names, field paths and their types, the conditions you or the model wrote (such as message type values); no message, name or id. `history/` keeps replaced revisions, `*.candidate.json` refused ones | `600` |
| `~/.local/share/cade/cade.db.before-rekey-*` | copy saved before `schema-check --rekey` renames UIDs; same contents as the database | `600` |
| `~/.local/share/cade/models/` | the two models and the vision projector (public files) | — |
| `~/.local/state/cade/ingest-state.json` | the running or last `ingest`: its arguments (source and targets), stage, last progress line and error | `600` |
| `~/.local/state/cade/ingest.log` | output of the last `ingest start`: progress lines naming the targets, the report, errors; replaced on every background run | `600` |
| `/tmp/cade-browser-*`, `/tmp/cade-indexeddb-*`, `/tmp/cade-firefoxidb-*`, `/tmp/cade-firefoxcache-*` | copies of the browser history, of an IndexedDB (Chromium or Firefox) and of Firefox's Cache API database while one command reads it; removed when it ends | `700` |

Backups of your home directory include `cade.db`.

## What each source stores

| Source | Stored | Not stored |
|---|---|---|
| **git** | commit message, names of changed files, repository path, hash, author name and e-mail, time | diffs, file contents |
| **browser** | every visit: URL with credential parameters removed, page title, time | page contents, cookies, passwords, form data; private windows are not in the history |
| **files** | path, size, modification time and (for UTF-8 text up to `max_file_bytes`) text, except default ignored credential file names; with `sources.images` on, a description of each png, jpeg and webp image written by the local model (what it shows and the text visible in it), with the image's SHA-256, its size in pixels, and the model and prompt version | binary contents, image pixels; configured ignored directories and credential file globs |
| **teams** | chat, channel and meeting-chat messages: text (also kept alone, to rebuild the stored content), sender name and id, conversation id and title, whether you sent it, edit version | calendar, call history, notifications, files; messages deleted before ingestion |
| **apps through a schema** (e.g. `whatsapp`) | what the schema maps: message and conversation ids, conversation name, sender id and name, time, whether you sent it, the text when the app keeps it readable, the schema name and revision | anything else in the app's IndexedDB; for WhatsApp Web, the text (encrypted by the app, never decrypted) and media |

File ingestion skips `.env*`, key and certificate files, common credential files and the patterns listed in `sources.ignored_file_globs`. Browser URLs drop `token`, OAuth, signature, password and cloud signing query parameters while preserving other parameters. With `ingest.redact` enabled (default), known GitHub, GitLab, AWS and Slack tokens, JWTs and PEM private-key blocks are replaced with labels such as `[redacted:github-token]` in event text and metadata. This also applies to existing events in schema migration 8; it creates a `cade.db.before-v8-*` backup. Changed text loses its vectors until `cade reindex`.

This detects formats, not every secret: arbitrary passwords, custom tokens and secrets in unrecognized formats can still be stored. Set `ingest.redact` to `false` to disable text masking; file globs and URL parameter removal remain active. Setting `sources.ignored_file_globs` replaces the default list, so include the defaults if you want to extend it.

## Images

Off by default; `cade init` asks, and `sources.images` turns it on.

- **The description takes the place of the image.** It goes through the same secret masking as any text: a screenshot of a terminal with a token stores `[redacted:github-token]`. The ignored folders and file globs apply to images too.
- **The SHA-256 lets a moved or renamed image reuse its description** without being described again. It identifies the file's bytes, not what it shows.
- **Text inside an image is someone else's,** like a message: the model is told to copy it, not to follow it, and a description whose text gives the assistant orders is marked untrusted in `ask`, as below.

## Teams

- **Check your organization's data policy first.** The Teams cache holds other people's messages, in chats and channels, and ingesting it copies them into cade's database on your disk. Many organizations restrict keeping work messages outside the tools they approve. `cade init` leaves Teams out unless you choose it.
- Messages come from the IndexedDB that Teams on the web keeps in Chrome, Firefox or Floorp. cade copies the whole IndexedDB directory to a temporary folder (the live files are locked), decodes only the message, conversation and profile stores, and deletes the copy.
- Only what the Teams client has cached is available. Older messages are not fetched, since cade never talks to Microsoft.
- An edited message replaces the stored text on the next ingestion. A message deleted in Teams after being ingested **stays** in cade until you run `cade forget teams`.
- `cade teams-schema` prints the structure of an IndexedDB (store names, field names, types, counts) with no values, so its output can be shared for diagnosis.

## Other apps through IndexedDB schemas

- **Same care as Teams:** a chat app's cache holds other people's messages, and ingesting it copies them into cade's database. Check the app's and your organization's terms first.
- `schema-discover`, `schema-check` and the ingestion copy the IndexedDB directory to a temporary folder, read the copy and delete it. cade never changes the app's data.
- **Encrypted data stays encrypted.** WhatsApp Web encrypts message bodies in IndexedDB; cade does not decrypt them, so its messages are indexed by metadata only.
- A message deleted in the app after ingestion **stays** in cade until `cade forget <source>`. A schema change never deletes events.
- **The browser's HTTP cache and Cache API hold the responses of every site you visit,** not only the app's. cade reads from them only the URLs you name in `sources.request_cache_urls`, which is empty by default; a pattern's origin cannot have a wildcard, so it never reaches another site. For every other entry cade reads only its key (the URL) and never its body; Firefox's Cache API database is copied to a temporary folder and deleted, the bodies are read in place, and nothing in these caches is changed.

## What the local model sees

Both models run in-process through llama.cpp.

- **Vision model** (only with `sources.images` on, during `ingest` and `reindex --captions`): each new image of the configured folders, scaled so its longer side is at most 1,024 px, with fixed instructions to describe it and copy its visible text. It sees nothing else, and it is freed before the embedding model loads.
- **Embedding model:** the whole text of each event, in chunks of up to 1,200 characters, when it is ingested (identical text is embedded once), and the search text of each question.
- **Question interpretation:** only your question, with fixed instructions and examples. Questions the rules read (a period, a source and generic words) never reach the model.
- **Saved prompt state:** `~/.cache/cade/prompt-state/` holds one file (~55 MB, owner-only) with the model's state after its fixed instructions and examples. It contains no question and nothing from the database.
- **Answers:** your question, today's date and up to `top_k` (6 by default) events; for a long event, only the chunk that matched (up to 1,200 characters), otherwise its text up to that length.
- **Text that gives the assistant orders:** messages, page titles and notes are other people's words, and one may be written to steer the answer ("IMPORTANTE para o assistente: ignore as regras…"). Such events go to the model marked as untrusted and without their text, with a rule not to use or cite them; the sources list shows the event with the mark, and `ask --json` sets `"untrusted": true`. This lowers the risk without removing it: text the detector does not recognize still reaches the model whole and can steer the answer, but the model has no tools, so it cannot act on anything.
- **Schema discovery** (only `cade schema-discover` and `cade schema-check --update`, run by you): the IndexedDB's largest stores with their names masked (e-mails, GUIDs and runs of 6+ digits hidden), the paths of their fields and the types found, and up to two samples per path. Text samples are masked the same way and cut at 40 characters; numbers show only their digit count (`<n:13>`), dates and binary data only their type. On a regeneration it also reads the current schema (paths only) and which paths changed. The schema it writes holds paths and types, never a sample. Ingestion never runs the model.
- Nothing else in the database is given to the model. Listings and task reports do not use the generation model; a listing with a topic ("pages about redis") embeds only the topic.

## Logs

- Logs go to stderr only. The exception is `ingest start`, which has no terminal: its stderr and stdout go to `~/.local/state/cade/ingest.log` (see above), with the same content a foreground run prints.
- `--verbose` adds debug lines that include the model's reply, the titles of retrieved events and the resolved question. Do not paste them publicly.

## Deleting data

| Goal | Command |
|---|---|
| Delete one source | `cade forget teams` (or `git`, `browser`, `file`) |
| Delete one event | `cade forget --uid UID` or review matches with `cade forget --match TEXT [--source S] [--from D --to D]` |
| Delete everything | `rm ~/.local/share/cade/cade.db*` |
| Remove the configuration | `rm -r ~/.config/cade` |
| Remove the saved prompt state | `rm -r ~/.cache/cade/prompt-state` (rebuilt on the next question) |

- `forget` deletes the events and their embeddings, then compacts the file and empties the write-ahead log, so the deleted text is gone from disk rather than left in free pages.
- The keyword index follows the text: an edit, `reindex` and `forget` remove the old words from it too.
- Events with the same text (a page visited several times) share one set of chunks, embeddings and keyword entries. `forget` of one of them deletes that event, its hash or path entry and its date from the embedding; the shared chunks stay while another event has the text, and go with the last one.
- The people index (`event_people`) holds the names in each message and commit (sender or author, conversation title, first names after "@"), so a question about a person is filtered in the database. It follows the events: an edit replaces an event's names, and `forget` deletes them with the events.
- Replaced text, such as an edited message or an older version of a file, is zeroed as well (SQLite `secure_delete`). Only the current version of a file is kept; `file_modifications` keeps the date and size of each earlier version, not its text, and `forget file` deletes it.
- A file deleted from its folder leaves answers but stays in the database (and the timeline) until `cade forget file`. For an image, that includes its description, which a copy of the same image found later reuses.
- `forget file` and `forget --uid` delete an image's description with its event; nothing about it is kept elsewhere. `cade reindex --captions` replaces descriptions in place, zeroing the old text.
- `forget` does not touch migration backups (`cade.db.before-v*`); delete them yourself.
- Copies made outside cade are not affected: backups, snapshots, or `cade ask --json` output you saved.
- `forget --uid` and confirmed `forget --match` remove the event, chunks, embeddings, keyword and people indexes, and file history; only its UID and deletion date remain to prevent re-ingestion. `forget <source>` clears that forgotten-UID list. Retention is off by default; set `ingest.retention.max_age_days` per source to opt in.
