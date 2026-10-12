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

## Building from source (FreeBSD)

FreeBSD amd64, from source only: there is no FreeBSD CI or release archive (#101).

```sh
pkg install go cmake ninja git sqlite3
go tool mage cpu        # CPU build
go tool mage models
go tool mage install
```

The tasks pass `CGO_CFLAGS="-I/usr/local/include -D__COSMOPOLITAN__"` and `CGO_LDFLAGS=-L/usr/local/lib`: `pkg` puts `sqlite3.h` under `/usr/local`, and the macro skips sqlite-vec typedefs that need a header FreeBSD does not include for it (`internal/devtasks/cgo_env.go`). Set them yourself to run `go build` or `go test` directly. llama.cpp is built with the system clang and linked with libc++.

**GPU:** NVIDIA ships no CUDA Toolkit for FreeBSD, so the GPU is used through Vulkan, which also runs AMD and Intel GPUs:

```sh
pkg install vulkan-loader vulkan-headers spirv-headers shaderc   # plus nvidia-driver, or drm-kmod for AMD and Intel
go tool mage vulkan     # Vulkan build; `cade version` says Vulkan
```

The NVIDIA driver must support the GPU: Pascal cards such as the MX250 or GTX 10xx need the 580 legacy branch (`pkg install nvidia-driver-580 nvidia-kmod-580`), which the current driver ignores, and the driver and kernel module must have the same version (`sysctl hw.nvidia.version`, `nvidia-smi`). On a GPU with little memory, lower `embedding.gpu_layers` and `generation.gpu_layers` so the models fit.

## Building from source (macOS, Apple Silicon)

Experimental: nobody on the project has a Mac, so this is built and tested only on GitHub's macOS runners (#53).

```sh
xcode-select --install   # the Command Line Tools: clang, the SDK with sqlite3.h
brew install go cmake ninja git
go tool mage cpu         # on Apple Silicon this is the Metal build: `cade version` says Metal
go tool mage models
go tool mage install
```

llama.cpp turns Metal and Accelerate on by default there, with the Metal shaders embedded in the binary. The models still run on the CPU by default (`gpu_layers: 0` on macOS): on GitHub's virtual Macs Metal produced garbage, and nobody has checked a real one. To try Metal, set `embedding.gpu_layers` and `generation.gpu_layers` to `-1`, and please report how it goes. The configuration is `~/Library/Application Support/cade/config.json`; the database and models stay under `~/.local/share/cade`. A downloaded release archive is not signed, so macOS blocks it until you run `xattr -d com.apple.quarantine cade`.

## Building from source (Windows, x86-64)

Experimental: nobody on the project has a Windows machine, so this is built and tested only on GitHub's Windows runners and, as cross-compiled binaries, under wine (#54). CPU only: cgo needs gcc, which MinGW provides, and CUDA does not build with MinGW.

1. Install [Go](https://go.dev/dl/), [Git for Windows](https://git-scm.com/download/win) (the `git` source calls `git`) and [MSYS2](https://www.msys2.org/).
2. In the MSYS2 UCRT64 shell, with Go on the `PATH`:

```sh
pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja mingw-w64-ucrt-x86_64-sqlite3
go tool mage cpu      # bin/cade.exe, linked statically: it needs no MinGW DLL to run
go tool mage models   # to ~/AppData/Local/cade/models
```

The configuration is `%APPDATA%\cade\config.json`; the database and models live in `%LOCALAPPDATA%\cade`, and paths are stored with `/` (`C:/Users/me/notes`), as on Linux. The release zip is not signed, so SmartScreen may warn before the first run. The image preview needs `chafa` on the `PATH` (`scoop install chafa`).

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
| `build` (default) | NVIDIA binary in `bin/cade`, as `cuda` |
| `cpu` | CPU binary in `bin/cade` |
| `cuda` | NVIDIA binary in `bin/cade` (CUDA Toolkit required) |
| `vulkan` | Vulkan binary in `bin/cade`, for GPUs without CUDA (Vulkan and SPIR-V headers and `glslc` required) |
| `install` / `uninstall` | copies `bin/cade` to `$DESTDIR$PREFIX/bin` (default `~/.local/bin`) / removes it |
| `dist` | release archive and SHA-256 in `dist/` |
| `llama` / `llamaCuda` / `llamaVulkan` | clones the pinned llama.cpp and builds its libraries (the other targets do it when needed) |
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
