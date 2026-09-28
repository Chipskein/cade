<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="200">
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

### Images (optional)

With `sources.images` on (`cade init` asks), `ingest` describes the png, jpeg and webp files of your folders with the local vision model (the `mmproj` that `go tool mage models` downloads): screenshots, whiteboard photos and diagrams are then found by what they show and by the text in them. Only the description is stored, never the pixels ([PRIVACY.md](PRIVACY.md)).

It costs about 1.7 s per screenshot on an RTX 3060 and 22 s on a 6-core CPU, so a folder of 1,000 screenshots takes ~30 min on a GPU and ~6 h on a CPU. Each `ingest` describes up to `ingest.max_images_per_run` (default 50) and leaves the rest for the next runs. `cade reindex --captions` describes them again after the model or the prompt changes.

Asking about the text inside an image (in Portuguese, "images where the characters say 'help me pay for divorce papers'"):

```console
$ cade ask "Imagens em que o personagens falam 'help me pay for divorce papers' "
Understood: answer · file · topic: personagens falam 'help me pay for divorce papers'
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

The cited image, found by the words in its caption box:

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: a game screenshot with the caption 'help me pay for divorce papers'" width="320">
</p>

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
| `eval` | `evalPlan`, `evalCaptions`, `evalRetrieval` and `evalInjection` with the real models |
| `evalCaptions` | describes the image fixtures (`testdata/images`) and checks the words each description must contain |
| `evalScale` / `evalRerank` | scale curve (`$SCALE`, report in `$SCALE_REPORT`) / reranking experiment (phase 17) |
| `bench` | latency and memory benchmarks |
| `fmt` / `fmtCheck` / `vet` / `lint` | gofmt, go vet, golangci-lint (`go tool mage print GOLANGCI_LINT_VERSION` is the pinned version) |
| `clean` | removes `bin/`, `dist/`, `coverage.out` and the llama.cpp builds |
| `print NAME` | prints a pinned value for CI cache keys (`LLAMA_TAG`, `LLAMA_CMAKE_FLAGS`, …) |

Settings are environment variables: `PREFIX`, `DESTDIR`, `MODELS_DIR`, `EMBEDDING_MODEL`, `GENERATION_MODEL`, `LLAMA_NATIVE` (`OFF` for a portable build), `CUDA_HOME`, `CUDA_ARCH`, `NVCC_CCBIN`, `EVAL_TIMEOUT` (default `1h`), `MODE`, `VERSION`. The evaluations and `bench` use the GPU when the CUDA Toolkit is installed; an empty `GO_TAGS` forces the CPU, e.g. `GO_TAGS= go tool mage bench`.
