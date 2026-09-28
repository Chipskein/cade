<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

[English](README.md) · **Português**

`cade` é uma CLI local-first para o seu histórico pessoal de atividades: commits git, histórico do navegador, arquivos, imagens e mensagens do Microsoft Teams, consultados por período ou em linguagem natural. Tudo roda na sua máquina (SQLite + sqlite-vec + llama.cpp) — veja [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md).

## Instalação

A partir do código-fonte (Linux; Go 1.27+, `gcc`, `cmake`, `ninja`, `git`):

```sh
go tool mage build     # ou `go tool mage cuda` para NVIDIA
go tool mage models    # baixa os modelos em ~/.local/share/cade/models
go tool mage install   # instala em ~/.local/bin
```

Binários prontos para Linux x86-64 estão na [página de releases](https://github.com/Chipskein/cade/releases); veja [Desenvolvimento](docs/DEVELOPMENT.pt-BR.md) para a verificação do checksum e os modelos.

## Uso

```sh
cade init
cade ingest all
cade timeline ontem
cade tasks hoje
cade ask "o que eu fiz ontem?"
```

### Imagens (opcional)

Ligue `sources.images` (o `cade init` pergunta) e o `ingest` descreve seus arquivos png, jpeg e webp com um modelo de visão local, para você encontrá-los pelo que mostram e pelo texto que têm. Só a descrição fica guardada, nunca os pixels.

- **Custo:** ~1,7 s por imagem numa RTX 3060, ~22 s numa CPU de 6 núcleos.
- **Lotes:** cada `ingest` descreve até `ingest.max_images_per_run` (padrão 50); o resto fica para a próxima execução.
- **Refazer:** `cade reindex --captions` depois de trocar o modelo ou o prompt.

Exemplo: encontrando uma imagem pelo texto dela:

```console
$ cade ask "Imagens em que o personagens falam 'help me pay for divorce papers' "
Understood: answer · file · topic: personagens falam 'help me pay for divorce papers'
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: captura de jogo com a legenda 'help me pay for divorce papers'" width="320">
</p>

## Desenvolvimento

```sh
go tool mage -l      # lista os alvos
go tool mage check   # o que a CI roda: fmtCheck, vet, lint e test
```

Alvos, variáveis de ambiente e detalhes de build: [docs/DEVELOPMENT.pt-BR.md](docs/DEVELOPMENT.pt-BR.md).

## Documentação

[Arquitetura](docs/ARCHITECTURE.md) · [Casos de uso](docs/USECASES.md) · [Configuração](docs/CONFIGURATION.pt-BR.md) · [Desenvolvimento](docs/DEVELOPMENT.pt-BR.md) · [Benchmarks](docs/BENCHMARKS.md) · [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.pt-BR.md)

Licença GPL-3.0-or-later ([LICENSE](LICENSE), [avisos de terceiros](THIRD_PARTY_NOTICES.md), [detalhes](docs/LICENSING.pt-BR.md)).
