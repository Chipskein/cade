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
- `gcc`, `cmake`, `ninja`, `curl`

```sh
make build      # build CPU
make cuda       # opcional: build NVIDIA (requer CUDA Toolkit)
make models     # baixa os modelos em ~/.local/share/cade/models
make install    # instala o cade em ~/.local/bin (altere com PREFIX=...)
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
make models
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

## Documentação

Para detalhes de implementação e guias aprofundados, veja:

- [Arquitetura](docs/ARCHITECTURE.md)
- [Casos de uso](docs/USECASES.md)
- [Referência de configuração](docs/CONFIGURATION.pt-BR.md)
- [Benchmarks](docs/BENCHMARKS.md)
- [Roadmap](docs/ROADMAP.md)
- [Changelog](CHANGELOG.pt-BR.md)

## Desenvolvimento

```sh
make test
```
