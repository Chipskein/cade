<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

**English** · [Português](README.pt-BR.md)

`cade` is a local-first CLI for your personal activity history: git commits, browser history, files, images and Microsoft Teams messages, queried by period or in natural language. Everything runs on your machine (SQLite + sqlite-vec + llama.cpp) — see [PRIVACY.md](PRIVACY.md).

## Install

From source (Linux; Go 1.27+, `gcc`, `cmake`, `ninja`, `git`):

```sh
go tool mage build     # NVIDIA (CUDA Toolkit); `go tool mage cpu` without a GPU
go tool mage models    # downloads the models to ~/.local/share/cade/models
go tool mage install   # installs to ~/.local/bin
```

Prebuilt Linux x86-64 binaries are on the [releases page](https://github.com/Chipskein/cade/releases).

Optional: install [`chafa`](https://github.com/hpjansson/chafa) to see a preview of cited images in `cade ask`.

## Platform support

| Source | Linux | macOS | Windows |
|--------|-------|-------|---------|
| git | ✓ tested | should work | not supported |
| browser | ✓ tested | should work | not supported |
| files | ✓ tested | should work | not supported |
| teams | ✓ tested | should work | not supported |
| images | ✓ tested | should work | not supported |
| image preview (`chafa`) | ✓ tested | should work (Homebrew) | not supported |

Linux x86-64 is the only CI and release target. macOS should work when built from source. Windows is not supported.

## Examples

```sh
cade init
cade ingest all
cade ingest start all   # long runs: detached, with CPU/GPU limits
cade ingest status      # also: pause, resume, stop
cade timeline yesterday
cade tasks today
cade ask "what did I work on yesterday?"
```

Finding an image by its text:

```console
$ cade ask "images where the characters say 'help me pay for divorce papers'"
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: a game screenshot with the caption 'help me pay for divorce papers'" width="320">
</p>

Finding an image by what it shows:

```console
$ cade ask "album cover with a blonde woman"
A imagem de capa de álbum com uma mulher loira foi encontrada em um arquivo acessado em 15 de setembro de 2026 [1].

Cited sources:
  [1] [file]    2026-09-15 10:49  /home/chipskein/Downloads/maxresdefault.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-album-example.jpg" alt="maxresdefault.jpg: a parody album cover showing a blonde woman" width="320">
</p>

## More information

| | |
|---|---|
| [Configuration](docs/CONFIGURATION.md) | All config fields, file ingestion behavior, how search works, Teams setup, background and scheduled ingestion. |
| [Use cases](docs/USECASES.md) | Practical query patterns and workflows. |
| [Architecture](docs/ARCHITECTURE.md) | How the code is organized and how components interact. |
| [Benchmarks](docs/BENCHMARKS.md) | Search latency, retrieval quality and image description cost. |
| [Development](docs/DEVELOPMENT.md) | Build targets, environment variables, CI. |
| [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.md) | What's planned and what changed. |

Licensed GPL-3.0-or-later ([LICENSE](LICENSE), [third-party notices](THIRD_PARTY_NOTICES.md), [details](docs/LICENSING.md)).
