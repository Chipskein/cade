<p align="center">
  <img src="assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="220">
</p>

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
- `gcc`, `cmake`, `ninja`, `git`

```sh
go tool mage build      # CPU build
go tool mage cuda       # optional NVIDIA build (CUDA Toolkit required)
go tool mage models     # downloads models to ~/.local/share/cade/models
go tool mage install    # installs cade to ~/.local/bin (override with PREFIX=...)
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
go tool mage models   # in a clone of this repository
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

`tasks` reports **PR opened** when local history shows a visit to the PR creation page. Offline, it cannot tell whether the PR was approved or merged. To use a project-specific tracker such as Proj4me, add a pattern under `tasks.task_url_patterns` in the config:

```json
"task_url_patterns": ["proj4\\.me/projects/(\\d+)/tasks/(\\d+)"]
```

## Documentation

For implementation details and in-depth guides, see:

- [Architecture](docs/ARCHITECTURE.md)
- [Use cases](docs/USECASES.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Benchmarks](docs/BENCHMARKS.md)
- [Roadmap](docs/ROADMAP.md)
- [Licensing](docs/LICENSING.md): GPL-3.0-or-later ([LICENSE](LICENSE), [third-party notices](THIRD_PARTY_NOTICES.md))
- [Changelog](CHANGELOG.md)

## Development

The development tasks are [Mage](https://magefile.org/) targets written in Go (`magefiles/`, logic in `internal/devtasks/`). Mage is a Go tool dependency of this module, so there is nothing to install: `go tool mage` runs it, and it is never linked into `cade`.

```sh
go tool mage -l     # lists the targets
go tool mage test   # unit tests
go tool mage check  # what CI runs: fmtCheck, vet, lint and test
```

| Target | What it does |
| --- | --- |
| `build` (default) | CPU binary in `bin/cade` |
| `cuda` | NVIDIA binary in `bin/cade` (CUDA Toolkit required) |
| `install` / `uninstall` | copies `bin/cade` to `$DESTDIR$PREFIX/bin` (default `~/.local/bin`) / removes it |
| `dist` | release archive and SHA-256 in `dist/` |
| `llama` / `llamaCuda` | clones the pinned llama.cpp and builds its libraries (the other targets do it when needed) |
| `models` | downloads the models to `$MODELS_DIR` (default `~/.local/share/cade/models`) |
| `test` / `cover` | unit tests / with the per-function coverage table |
| `fuzz` | the parser fuzz targets, `$FUZZTIME` each (default `30s`) |
| `testModels` | every test, including the llama.cpp binding against the real models |
| `eval` | `evalPlan`, `evalRetrieval` and `evalInjection` with the real models |
| `evalScale` / `evalRerank` | scale curve (`$SCALE`, report in `$SCALE_REPORT`) / reranking experiment (phase 17) |
| `bench` | latency and memory benchmarks |
| `fmt` / `fmtCheck` / `vet` / `lint` | gofmt, go vet, golangci-lint (`go tool mage print GOLANGCI_LINT_VERSION` is the pinned version) |
| `clean` | removes `bin/`, `dist/`, `coverage.out` and the llama.cpp builds |
| `print NAME` | prints a pinned value for CI cache keys (`LLAMA_TAG`, `LLAMA_CMAKE_FLAGS`, …) |

Settings are environment variables: `PREFIX`, `DESTDIR`, `MODELS_DIR`, `EMBEDDING_MODEL`, `GENERATION_MODEL`, `LLAMA_NATIVE` (`OFF` for a portable build), `CUDA_HOME`, `CUDA_ARCH`, `NVCC_CCBIN`, `EVAL_TIMEOUT` (default `1h`), `MODE`, `VERSION`. The evaluations and `bench` use the GPU when the CUDA Toolkit is installed; an empty `GO_TAGS` forces the CPU, e.g. `GO_TAGS= go tool mage bench`.
