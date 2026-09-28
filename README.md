<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

**English** · [Português](README.pt-BR.md)

`cade` is a local-first CLI for your personal activity history: git commits, browser history, files, images and Microsoft Teams messages, queried by period or in natural language. Everything runs on your machine (SQLite + sqlite-vec + llama.cpp) — see [PRIVACY.md](PRIVACY.md).

## Install

From source (Linux; Go 1.27+, `gcc`, `cmake`, `ninja`, `git`):

```sh
go tool mage build     # or `go tool mage cuda` for NVIDIA
go tool mage models    # downloads the models to ~/.local/share/cade/models
go tool mage install   # installs to ~/.local/bin
```

Prebuilt Linux x86-64 binaries are on the [releases page](https://github.com/Chipskein/cade/releases); see [Development](docs/DEVELOPMENT.md) for the checksum and model steps.

## Usage

```sh
cade init
cade ingest all
cade timeline yesterday
cade tasks today
cade ask "what did I work on yesterday?"
```

### Images (optional)

Enable `sources.images` (`cade init` asks) and `ingest` describes your png, jpeg and webp files with a local vision model, so you can find them by what they show and the text in them. Only the description is stored, never the pixels.

- **Cost:** ~1.7 s per image on an RTX 3060, ~22 s on a 6-core CPU.
- **Batches:** each `ingest` describes up to `ingest.max_images_per_run` (default 50); the rest wait for the next run.
- **Redo:** `cade reindex --captions` after changing the model or prompt.

Example: finding an image by its text ("images where the characters say …"):

```console
$ cade ask "Imagens em que o personagens falam 'help me pay for divorce papers' "
Understood: answer · file · topic: personagens falam 'help me pay for divorce papers'
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: a game screenshot with the caption 'help me pay for divorce papers'" width="320">
</p>

## Development

```sh
go tool mage -l      # lists the targets
go tool mage check   # what CI runs: fmtCheck, vet, lint and test
```

Targets, environment variables and build details: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Documentation

[Architecture](docs/ARCHITECTURE.md) · [Use cases](docs/USECASES.md) · [Configuration](docs/CONFIGURATION.md) · [Development](docs/DEVELOPMENT.md) · [Benchmarks](docs/BENCHMARKS.md) · [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.md)

Licensed GPL-3.0-or-later ([LICENSE](LICENSE), [third-party notices](THIRD_PARTY_NOTICES.md), [details](docs/LICENSING.md)).
