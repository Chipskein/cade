# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

**English** · [Português](README.pt-BR.md)

`cade` is a local-first CLI for personal activity history.
It ingests git commits, browser history, files, and Microsoft Teams messages, then lets you query by period (`timeline`, `tasks`) or natural language (`ask`).

- Runs locally (SQLite + sqlite-vec + llama.cpp)
- No server required
- Privacy details: [PRIVACY.md](PRIVACY.md)

## Install

### From source (Linux)

Requirements:
- Go 1.27+
- `gcc`, `cmake`, `ninja`, `curl`

```sh
make build      # CPU build
make cuda       # optional NVIDIA build (CUDA Toolkit required)
make models     # downloads models to ~/.local/share/cade/models
make install    # installs cade to ~/.local/bin (override with PREFIX=...)
```

### From release binary (Linux x86-64, CPU)

```sh
V=v0.0.0
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz.sha256
sha256sum -c cade-$V-linux-amd64-cpu.tar.gz.sha256

tar xzf cade-$V-linux-amd64-cpu.tar.gz
install -Dm755 cade-$V-linux-amd64-cpu/cade ~/.local/bin/cade
```

Then run:

```sh
make models
cade init
cade doctor
cade ingest all
```

## Basic usage

```sh
cade timeline yesterday
cade ask "what did I work on yesterday?"
cade tasks today
```

## Documentation

For implementation details and in-depth guides, see:

- [Architecture](docs/ARCHITECTURE.md)
- [Use cases](docs/USECASES.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Benchmarks](docs/BENCHMARKS.md)
- [Roadmap](docs/ROADMAP.md)
- [Changelog](CHANGELOG.md)

## Development

```sh
make test
```
