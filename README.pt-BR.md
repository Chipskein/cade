<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

[English](README.md) · **Português**

`cade` é uma CLI local-first para histórico pessoal de atividades.
Ela ingere commits git, histórico de navegador, arquivos e mensagens do Microsoft Teams, e permite consultar por período (`timeline`, `tasks`) ou linguagem natural (`ask`).

- Roda localmente (SQLite + sqlite-vec + llama.cpp)
- Não requer servidor
- Privacidade: [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md)

## Instalação

### A partir do código-fonte (Linux)

Requisitos:
- Go 1.27+
- `gcc`, `cmake`, `ninja`, `git`

```sh
go tool mage build      # build CPU
go tool mage cuda       # opcional: build NVIDIA (requer CUDA Toolkit)
go tool mage models     # baixa os modelos em ~/.local/share/cade/models
go tool mage install    # instala o cade em ~/.local/bin (altere com PREFIX=...)
```

### A partir do binário de release (Linux x86-64, CPU)

```sh
V=v0.0.0
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz
curl -LO https://github.com/Chipskein/cade/releases/download/$V/cade-$V-linux-amd64-cpu.tar.gz.sha256
sha256sum -c cade-$V-linux-amd64-cpu.tar.gz.sha256

tar xzf cade-$V-linux-amd64-cpu.tar.gz
install -Dm755 cade-$V-linux-amd64-cpu/cade ~/.local/bin/cade
```

Depois execute:

```sh
go tool mage models   # num clone deste repositório
cade init
cade doctor
cade ingest all
```

## Uso básico

```sh
cade timeline ontem
cade ask "o que eu fiz ontem?"
cade tasks hoje
```

`tasks` mostra **PR aberto** quando o histórico local registra uma visita à página de criação do PR. Sem rede, não dá para saber se ele foi aprovado ou mergeado. Para usar um rastreador específico do projeto, como o Proj4me, adicione um padrão em `tasks.task_url_patterns` na configuração:

```json
"task_url_patterns": ["proj4\\.me/projects/(\\d+)/tasks/(\\d+)"]
```

### Imagens (opcional)

Com `sources.images` ligado (o `cade init` pergunta), o `ingest` descreve os arquivos png, jpeg e webp das suas pastas com o modelo de visão local (o `mmproj` que o `go tool mage models` baixa): capturas de tela, fotos de quadro e diagramas passam a ser encontrados pelo que mostram e pelo texto que têm. Só a descrição fica guardada, nunca os pixels ([PRIVACY.pt-BR.md](PRIVACY.pt-BR.md)).

Custa cerca de 1,7 s por captura numa RTX 3060 e 22 s numa CPU de 6 núcleos: uma pasta com 1.000 capturas leva ~30 min com GPU e ~6 h em CPU. Cada `ingest` descreve até `ingest.max_images_per_run` (padrão 50) e deixa o resto para as próximas execuções. `cade reindex --captions` descreve de novo quando o modelo ou o prompt mudam.

Perguntando pelo texto dentro de uma imagem:

```console
$ cade ask "Imagens em que o personagens falam 'help me pay for divorce papers' "
Understood: answer · file · topic: personagens falam 'help me pay for divorce papers'
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

A imagem citada, encontrada pelas palavras da legenda:

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: captura de jogo com a legenda 'help me pay for divorce papers'" width="320">
</p>

## Documentação

Para detalhes de implementação e guias aprofundados, veja:

- [Arquitetura](docs/ARCHITECTURE.md)
- [Casos de uso](docs/USECASES.md)
- [Referência de configuração](docs/CONFIGURATION.pt-BR.md)
- [Benchmarks](docs/BENCHMARKS.md)
- [Roadmap](docs/ROADMAP.md)
- [Licenciamento](docs/LICENSING.pt-BR.md): GPL-3.0-or-later ([LICENSE](LICENSE), [avisos de terceiros](THIRD_PARTY_NOTICES.md))
- [Changelog](CHANGELOG.pt-BR.md)

## Desenvolvimento

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

As configurações são variáveis de ambiente: `PREFIX`, `DESTDIR`, `MODELS_DIR`, `EMBEDDING_MODEL`, `GENERATION_MODEL`, `LLAMA_NATIVE` (`OFF` para um build portátil), `CUDA_HOME`, `CUDA_ARCH`, `NVCC_CCBIN`, `EVAL_TIMEOUT` (padrão `1h`), `MODE`, `VERSION`. As avaliações e o `bench` usam a GPU quando o CUDA Toolkit está instalado; `GO_TAGS` vazio força a CPU, por exemplo `GO_TAGS= go tool mage bench`.
