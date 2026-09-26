# Privacy

**English** · [Português](PRIVACY.pt-BR.md)

cade reads your commits, browser history, files and Teams messages, so it holds some of the most personal data on your machine. This page lists what it stores, where, what the local model sees, and how to delete it.

## In short

- **Nothing leaves the machine at run time.** The binary has no network code: Go's `net` and `net/http` are not linked, and llama.cpp is built without its downloader. There is no telemetry.
- **Network is used only at build time:** `make llama` clones llama.cpp from GitHub and `make models` downloads the two models from Hugging Face.
- **Everything is in one SQLite file**, readable only by your user account (mode `600`, directory `700`).
- **The database is not encrypted.** Anyone logged in as you, or with your disk, can read it. Use disk encryption.

## Where data lives

| Path | Contents | Mode |
|---|---|---|
| `~/.local/share/cade/cade.db` (+ `-wal`, `-shm`) | events, their text and embeddings | `600` |
| `~/.config/cade/config.json` | which repositories, histories, directories and Teams profiles to read | `600` |
| `~/.local/share/cade/models/` | the two models (public files) | — |
| `/tmp/cade-browser-*`, `/tmp/cade-indexeddb-*` | copies of the browser history and Teams IndexedDB while one ingestion runs; removed when it ends | `700` |

Backups of your home directory include `cade.db`.

## What each source stores

| Source | Stored | Not stored |
|---|---|---|
| **git** | commit message, names of changed files, repository path, hash, author name and e-mail, time | diffs, file contents |
| **browser** | every visit in the history file: URL (with its query string, which sometimes carries tokens), page title, time | page contents, cookies, passwords, form data; private windows are not in the history |
| **files** | for each file under the configured directories: path, size, modification time and, for UTF-8 text files up to `max_file_bytes` (256 KB), the **whole text** | binary files' contents; directories in `ignored_dir_names` (`.git`, `node_modules`…) |
| **teams** | chat, channel and meeting-chat messages: text, sender name and id, conversation id and title, whether you sent it, edit version | calendar, call history, notifications, files; messages deleted before ingestion |

Choose `directories` with care: a `.env` or a notes file with passwords under them is stored as text.

## Teams

- Messages come from the IndexedDB that Teams on the web keeps in Chrome. cade copies the whole IndexedDB directory to a temporary folder (the live files are locked), decodes only the message, conversation and profile stores, and deletes the copy.
- Only what the Teams client has cached is available. Older messages are not fetched, since cade never talks to Microsoft.
- An edited message replaces the stored text on the next ingestion. A message deleted in Teams after being ingested **stays** in cade until you run `cade forget teams`.
- `cade teams-schema` prints the structure of an IndexedDB (store names, field names, types, counts) with no values, so its output can be shared for diagnosis.

## What the local model sees

Both models run in-process through llama.cpp.

- **Embedding model:** the text of each event (first 8,000 characters) when it is ingested, and the search text of each question.
- **Question interpretation:** only your question, with fixed instructions and examples.
- **Answers:** your question, today's date and up to `top_k` (8) events, each cut to 700 characters.
- Nothing else in the database is given to the model. Listings and task reports do not use the generation model; a listing with a topic ("pages about redis") embeds only the topic.

## Logs

- Logs go to stderr only; nothing is written to a log file.
- `--verbose` adds debug lines that include the model's reply, the titles of retrieved events and the resolved question. Do not paste them publicly.

## Deleting data

| Goal | Command |
|---|---|
| Delete one source | `cade forget teams` (or `git`, `browser`, `file`) |
| Delete everything | `rm ~/.local/share/cade/cade.db*` |
| Remove the configuration | `rm -r ~/.config/cade` |

- `forget` deletes the events and their embeddings, then compacts the file and empties the write-ahead log, so the deleted text is gone from disk rather than left in free pages.
- Replaced text, such as an edited message, is zeroed as well (SQLite `secure_delete`).
- Copies made outside cade are not affected: backups, snapshots, or `cade ask --json` output you saved.
- There is no command to delete a single event.
