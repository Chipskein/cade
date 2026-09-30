<p align="center">
  <img src="../assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Desenvolvimento

[English](DEVELOPMENT.md) · **Português**

## Compilando a partir do código-fonte (Linux)

Requisitos: Go 1.27+, `gcc`, `cmake`, `ninja`, `git`.

```sh
go tool mage build      # build CPU
go tool mage cuda       # opcional: build NVIDIA (requer CUDA Toolkit)
go tool mage models     # baixa os modelos em ~/.local/share/cade/models
go tool mage install    # instala o cade em ~/.local/bin (altere com PREFIX=...)
```

O `models` baixa o modelo de embedding, o de geração e o projetor de visão dele (`mmproj`), que descreve as imagens quando `sources.images` está ligado.

Opcional em tempo de execução: [`chafa`](https://github.com/hpjansson/chafa) (ex.: `pacman -S chafa`, `apt install chafa`, `brew install chafa`). O `cade ask` o procura no `PATH` e desenha cada png, jpeg ou webp citado logo abaixo da citação, só quando a saída padrão é um terminal. Sem ele, ou se ele falhar, a citação sai como antes. Ele não é ligado ao binário: compilá-lo exige meson e glib, um conjunto de ferramentas mais pesado que o do llama.cpp para um recurso só visual.

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
| `build` (padrão) | binário CPU em `bin/cade` |
| `cuda` | binário NVIDIA em `bin/cade` (requer CUDA Toolkit) |
| `install` / `uninstall` | copia `bin/cade` para `$DESTDIR$PREFIX/bin` (padrão `~/.local/bin`) / remove |
| `dist` | arquivo de release e SHA-256 em `dist/` |
| `llama` / `llamaCuda` | clona o llama.cpp fixado e compila as bibliotecas (os outros alvos fazem isso quando preciso) |
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
