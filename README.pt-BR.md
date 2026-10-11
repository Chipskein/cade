<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

[English](README.md) · **Português**

`cade` é uma CLI local-first para o seu histórico pessoal de atividades: commits git, histórico do navegador, arquivos, imagens e mensagens do Microsoft Teams, consultados por período ou em linguagem natural. Tudo roda na sua máquina (SQLite + sqlite-vec + llama.cpp) — veja [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md).

## Instalação

A partir do código-fonte (Linux ou FreeBSD; Go 1.27+, um compilador C/C++, `cmake`, `ninja`, `git`; no FreeBSD, veja o [DEVELOPMENT](docs/DEVELOPMENT.pt-BR.md#compilando-a-partir-do-código-fonte-freebsd)):

```sh
go tool mage build     # NVIDIA (CUDA Toolkit); `go tool mage cpu` sem GPU
go tool mage models    # baixa os modelos em ~/.local/share/cade/models
go tool mage install   # instala em ~/.local/bin
```

Binários prontos para Linux x86-64, e um experimental para macOS em Apple Silicon, estão na [página de releases](https://github.com/Chipskein/cade/releases).

Opcional: instale o [`chafa`](https://github.com/hpjansson/chafa) para ver uma prévia das imagens citadas no `cade ask`.

## Plataformas suportadas

| Fonte | Linux | FreeBSD | macOS | Windows |
|-------|-------|---------|-------|---------|
| git | ✓ testado | ✓ testado (do código-fonte) | experimental (só CI) | não suportado |
| browser | ✓ testado | ✓ testado (do código-fonte) | experimental (só CI) | não suportado |
| arquivos | ✓ testado | ✓ testado (do código-fonte) | experimental (só CI) | não suportado |
| teams | ✓ testado | ✓ testado (do código-fonte) | experimental (só CI) | não suportado |
| imagens | ✓ testado | ✓ testado (do código-fonte) | experimental (só CI) | não suportado |
| prévia de imagens (`chafa`) | ✓ testado | deve funcionar (`pkg install chafa`) | deve funcionar (Homebrew) | não suportado |

Linux x86-64 é o alvo principal de CI e releases. O macOS em Apple Silicon (Metal) tem um arquivo de release experimental, compilado e testado só nos Macs do GitHub ([#53](https://github.com/Chipskein/cade/issues/53)). O FreeBSD amd64 compila do código-fonte, em CPU ou na GPU pelo Vulkan ([#101](https://github.com/Chipskein/cade/issues/101)), sem CI; foi testado no FreeBSD 15.1. Windows não é suportado.

## Exemplos

```sh
cade init
cade ingest all
cade ingest start all   # execuções longas: fora do terminal, com limites de CPU/GPU
cade ingest status      # também: pause, resume, stop
cade timeline ontem
cade timeline --around UID   # 2h antes e depois de um evento (UID do `ask --json`)
cade tasks hoje
cade ask "o que eu fiz ontem?"
cade compact            # depois de um forget grande: devolve o espaço dos vetores apagados
cade export > cade.jsonl     # tudo o que o cade guarda, um evento por linha
```

Qualquer outro aplicativo que guarda os dados no IndexedDB do navegador (Chrome, Firefox, Floorp), sem código próprio: o modelo local escreve um schema uma vez, e a ingestão lê com ele.

```sh
cade schema-discover --name whatsapp ~/.floorp/<perfil>/storage/default/https+++web.whatsapp.com/idb
cade ingest whatsapp
cade schema-check --update  # quando o aplicativo muda o jeito de guardar os dados
```

O WhatsApp Web guarda o texto das mensagens cifrado, então ele é indexado só por metadados. O banco do WhatsApp para Android, depois que você decifra o backup, completa o texto e traz as mensagens mais antigas:

```sh
cade ingest whatsapp-backup ~/whatsapp/msgstore.db
```

Encontrando uma imagem pelo texto dela:

```console
$ cade ask "imagens em que os personagens falam 'help me pay for divorce papers'"
A imagem em que o personagem diz "help me pay for divorce papers" é a [1].

Cited sources:
  [1] [file]    2026-09-08 18:56  /home/chipskein/Downloads/HRrl-KYbIAAINeq.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-example.jpg" alt="HRrl-KYbIAAINeq.jpg: captura de jogo com a legenda 'help me pay for divorce papers'" width="320">
</p>

Encontrando uma imagem pelo que ela mostra:

```console
$ cade ask "álbum com mulher loira na capa"
A imagem de capa de álbum com uma mulher loira foi encontrada em um arquivo acessado em 15 de setembro de 2026 [1].

Cited sources:
  [1] [file]    2026-09-15 10:49  /home/chipskein/Downloads/maxresdefault.jpg
```

<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/ask-images-album-example.jpg" alt="maxresdefault.jpg: paródia de capa de álbum com uma mulher loira" width="320">
</p>

## Mais informações

| | |
|---|---|
| [Configuração](docs/CONFIGURATION.pt-BR.md) | Todos os campos de config, comportamento da ingestão de arquivos, como a busca funciona, configuração do Teams, outros aplicativos por schemas de IndexedDB, ingestão em segundo plano e agendada. |
| [Casos de uso](docs/USECASES.md) | Padrões de consulta e fluxos de trabalho práticos. |
| [Arquitetura](docs/ARCHITECTURE.md) | Como o código está organizado e como os componentes interagem. |
| [Benchmarks](docs/BENCHMARKS.pt-BR.md) | Latência de busca, qualidade da recuperação e custo da descrição de imagens. |
| [Desenvolvimento](docs/DEVELOPMENT.pt-BR.md) | Alvos de build, variáveis de ambiente, CI. |
| [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.pt-BR.md) | O que está planejado e o que mudou. |

Licença GPL-3.0-or-later ([LICENSE](LICENSE), [avisos de terceiros](THIRD_PARTY_NOTICES.md), [detalhes](docs/LICENSING.pt-BR.md)).
