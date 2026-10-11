<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Desenvolvimento

[English](DEVELOPMENT.md) · **Português**

## Compilando a partir do código-fonte (Linux)

Requisitos: Go 1.27+, `gcc`, `cmake`, `ninja`, `git`.

```sh
go tool mage build      # build padrão: NVIDIA (requer CUDA Toolkit); o mesmo que `cuda`
go tool mage cpu        # build CPU, para máquinas sem CUDA
go tool mage models     # baixa os modelos em ~/.local/share/cade/models
go tool mage install    # instala o bin/cade (compilando o padrão se faltar) em ~/.local/bin (altere com PREFIX=...)
```

O `models` baixa o modelo de embedding, o de geração e o projetor de visão dele (`mmproj`), que descreve as imagens quando `sources.images` está ligado.

Opcional em tempo de execução: [`chafa`](https://github.com/hpjansson/chafa) (ex.: `pacman -S chafa`, `apt install chafa`, `brew install chafa`). O `cade ask` o procura no `PATH` e desenha cada png, jpeg ou webp citado logo abaixo da citação, só quando a saída padrão é um terminal. Sem ele, ou se ele falhar, a citação sai como antes. Ele não é ligado ao binário: compilá-lo exige meson e glib, um conjunto de ferramentas mais pesado que o do llama.cpp para um recurso só visual.

## Compilando a partir do código-fonte (FreeBSD)

FreeBSD amd64, só do código-fonte: não há CI nem arquivo de release para FreeBSD (#101).

```sh
pkg install go cmake ninja git sqlite3
go tool mage cpu        # build CPU
go tool mage models
go tool mage install
```

As tarefas passam `CGO_CFLAGS="-I/usr/local/include -D__COSMOPOLITAN__"` e `CGO_LDFLAGS=-L/usr/local/lib`: o `pkg` põe o `sqlite3.h` em `/usr/local`, e o macro pula typedefs do sqlite-vec que dependem de um header que o FreeBSD não inclui por ele (`internal/devtasks/cgo_env.go`). Defina-as você mesmo para rodar `go build` ou `go test` direto. O llama.cpp é compilado com o clang do sistema e ligado à libc++.

**GPU:** a NVIDIA não distribui o CUDA Toolkit para FreeBSD, então a GPU é usada pelo Vulkan, que também roda GPUs AMD e Intel:

```sh
pkg install vulkan-loader vulkan-headers spirv-headers shaderc   # mais o nvidia-driver, ou o drm-kmod para AMD e Intel
go tool mage vulkan     # build Vulkan; o `cade version` diz Vulkan
```

O driver da NVIDIA precisa suportar a GPU: placas Pascal como a MX250 ou as GTX 10xx precisam do ramo legado 580 (`pkg install nvidia-driver-580 nvidia-kmod-580`), que o driver atual ignora, e o driver e o módulo do kernel precisam ter a mesma versão (`sysctl hw.nvidia.version`, `nvidia-smi`). Numa GPU com pouca memória, diminua `embedding.gpu_layers` e `generation.gpu_layers` para os modelos caberem.

## Instalando o binário de release (Linux x86-64, CPU)

```sh
V=v0.0.0
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz.sha256
sha256sum -c cade-$V-linux-amd64-cpu.tar.gz.sha256

tar xzf cade-$V-linux-amd64-cpu.tar.gz
install -Dm755 cade-$V-linux-amd64-cpu/cade ~/.local/bin/cade
```

Os modelos não vêm com o binário; baixe-os num clone deste repositório e confira a instalação:

```sh
go tool mage models
cade init
cade doctor
```

## Alvos do Mage

As tarefas de desenvolvimento são alvos do [Mage](https://magefile.org/) escritos em Go (`magefiles/`, lógica em `internal/devtasks/`). O Mage é uma dependência de ferramenta (`tool`) deste módulo, então não há o que instalar: `go tool mage` o executa, e ele nunca entra no binário do `cade`.

```sh
go tool mage -l     # lista os alvos
go tool mage test   # testes unitários
go tool mage check  # o que a CI roda: fmtCheck, vet, lint e test
```

| Alvo | O que faz |
| --- | --- |
| `build` (padrão) | binário NVIDIA em `bin/cade`, como o `cuda` |
| `cpu` | binário CPU em `bin/cade` |
| `cuda` | binário NVIDIA em `bin/cade` (requer CUDA Toolkit) |
| `vulkan` | binário Vulkan em `bin/cade`, para GPUs sem CUDA (requer os headers do Vulkan e do SPIR-V e o `glslc`) |
| `install` / `uninstall` | copia `bin/cade` para `$DESTDIR$PREFIX/bin` (padrão `~/.local/bin`) / remove |
| `dist` | arquivo de release e SHA-256 em `dist/` |
| `llama` / `llamaCuda` / `llamaVulkan` | clona o llama.cpp fixado e compila as bibliotecas (os outros alvos fazem isso quando preciso) |
| `models` | baixa os modelos em `$MODELS_DIR` (padrão `~/.local/share/cade/models`) |
| `test` / `cover` | testes unitários / com a tabela de cobertura por função |
| `fuzz` | os alvos de fuzz dos leitores, `$FUZZTIME` cada (padrão `30s`) |
| `testModels` | todos os testes, inclusive o binding do llama.cpp com os modelos reais |
| `eval` | `evalPlan`, `evalCaptions`, `evalRetrieval` e `evalInjection` com os modelos reais |
| `evalCaptions` | descreve as imagens de teste (`testdata/images`) e confere as palavras que cada descrição precisa ter |
| `evalScale` / `evalRerank` | curva de escala (`$SCALE`, relatório em `$SCALE_REPORT`) / experimento de reranking (fase 17) |
| `bench` | benchmarks de latência e memória |
| `fmt` / `fmtCheck` / `vet` / `lint` | gofmt, go vet, golangci-lint (`go tool mage print GOLANGCI_LINT_VERSION` é a versão fixada) |
| `clean` | remove `bin/`, `dist/`, `coverage.out` e os builds do llama.cpp |
| `print NOME` | imprime um valor fixado para chaves de cache da CI (`LLAMA_TAG`, `LLAMA_CMAKE_FLAGS`, …) |

## Variáveis de ambiente

`PREFIX`, `DESTDIR`, `MODELS_DIR`, `EMBEDDING_MODEL`, `GENERATION_MODEL`, `LLAMA_NATIVE` (`OFF` para um build portátil), `CUDA_HOME`, `CUDA_ARCH`, `NVCC_CCBIN`, `EVAL_TIMEOUT` (padrão `1h`), `MODE`, `VERSION`.

As avaliações e o `bench` usam a GPU quando o CUDA Toolkit está instalado; `GO_TAGS` vazio força a CPU, por exemplo `GO_TAGS= go tool mage bench`.
