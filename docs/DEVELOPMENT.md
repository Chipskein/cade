<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Development

**English** · [Português](DEVELOPMENT.pt-BR.md)

## Building from source (Linux)

Requirements: Go 1.27+, `gcc`, `cmake`, `ninja`, `git`.

```sh
go tool mage build      # default build: NVIDIA (CUDA Toolkit required); same as `cuda`
go tool mage cpu        # CPU build, for machines without CUDA
go tool mage models     # downloads the models to ~/.local/share/cade/models
go tool mage install    # installs bin/cade (building the default one if missing) to ~/.local/bin (override with PREFIX=...)
```

`models` downloads the embedding model, the generation model and its vision projector (`mmproj`), which describes images when `sources.images` is on.

Optional at runtime: [`chafa`](https://github.com/hpjansson/chafa) (e.g. `pacman -S chafa`, `apt install chafa`, `brew install chafa`). `cade ask` looks it up on `PATH` and draws each cited png, jpeg or webp under its citation, only when stdout is a terminal. Without it, or when it fails, the citation is printed as before. It is not linked into the binary: building it needs meson and glib, a heavier toolchain than llama.cpp's for a cosmetic feature.

## Installing a release binary (Linux x86-64, CPU)

```sh
V=v0.0.0
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz.sha256
sha256sum -c cade-$V-linux-amd64-cpu.tar.gz.sha256

tar xzf cade-$V-linux-amd64-cpu.tar.gz
install -Dm755 cade-$V-linux-amd64-cpu/cade ~/.local/bin/cade
```

The models are not shipped with the binary; download them from a clone of this repository, then check the setup:

```sh
go tool mage models
cade init
cade doctor
```

## Mage targets

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

## Environment variables

`PREFIX`, `DESTDIR`, `MODELS_DIR`, `EMBEDDING_MODEL`, `GENERATION_MODEL`, `LLAMA_NATIVE` (`OFF` for a portable build), `CUDA_HOME`, `CUDA_ARCH`, `NVCC_CCBIN`, `EVAL_TIMEOUT` (default `1h`), `MODE`, `VERSION`.

The evaluations and `bench` use the GPU when the CUDA Toolkit is installed; an empty `GO_TAGS` forces the CPU, e.g. `GO_TAGS= go tool mage bench`.
