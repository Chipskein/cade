# Licenciamento

[English](LICENSING.md) · **Português**

Revisão da licença do cade e do que entra no binário `cade`, feita para a [#10](https://github.com/Chipskein/cade/issues/10) em 2026-09-28. É uma revisão de engenharia, não uma consultoria jurídica.

## Decisão

O cade passa da **GPLv2** para a **GPLv3 ou posterior** (SPDX `GPL-3.0-or-later`), a partir da v0.1.0. A v0.0.0 continua sob a GPLv2 para quem a recebeu.

- **Nada obriga a troca:** todos os componentes do binário têm licença permissiva e funcionam com qualquer uma das versões.
- **A GPLv2 fecha portas de que o roteiro precisa:** ela é incompatível com a Apache-2.0 e com código (A)GPLv3. A busca em PDFs do roteiro precisa escolher uma biblioteca de extração e renderização, e boas candidatas são Apache-2.0 (pdfium) ou AGPL-3.0 (MuPDF). Muitas bibliotecas Go também são Apache-2.0.
- **A troca é possível agora:** todos os commits são do dono do projeto (inclusive os do agente Copilot nos PRs do próprio dono). Não há contribuidores externos a consultar; se houvesse, a troca dependeria do consentimento de cada um.
- **"ou posterior"** segue o aviso que a FSF recomenda. O `LICENSE` antigo trazia o texto da GPLv2 sem aviso de versão, o que deixava em aberto se era "only" ou "or later"; o identificador SPDX tira essa dúvida.

## Estado antes da troca

- O `LICENSE` tinha o texto da GPLv2 (junho de 1991), sem aviso nos fontes, e o `THIRD_PARTY_NOTICES.md` e o CHANGELOG diziam "GPLv2".
- O arquivo de release (`go tool mage dist`) leva o `LICENSE` e o `THIRD_PARTY_NOTICES.md` junto do binário.

## O que está no binário

Levantado com `go version -m bin/cade`, pelo `go.mod` e pelos `LDFLAGS` do cgo em `internal/llm/llamacpp`.

| Componente | Licença | GPLv2 | GPLv3 |
|---|---|---|---|
| Biblioteca padrão e runtime do Go | BSD-3-Clause | sim | sim |
| llama.cpp e ggml (`b11195`) | MIT | sim | sim |
| Bibliotecas que o llama.cpp embute em `mtmd` e `vendor-hash` (stb_image, miniaudio, xxHash, rotate-bits, sha1, sha256, sheredom/subprocess) | MIT, MIT-0, BSD-2-Clause, domínio público, Unlicense | sim | sim |
| mattn/go-sqlite3 v1.14.52 | MIT | sim | sim |
| SQLite (embutido no go-sqlite3) | domínio público | sim | sim |
| sqlite-vec via sqlite-vec-go-bindings v0.1.6 | MIT ou Apache-2.0, usado sob MIT | sim, só sob MIT | sim, sob qualquer uma |
| klauspost/compress v1.20.1 (`snappy`, `s2`, `gzip`, `flate`, `zstd`) | BSD-3-Clause | sim | sim |
| andybalholm/brotli v1.2.6 | MIT | sim | sim |
| golang.org/x/image v0.46.0 (`webp`, `draw`) | BSD-3-Clause | sim | sim |

As bibliotecas embutidas no llama.cpp não estavam no `THIRD_PARTY_NOTICES.md` antes desta revisão; agora estão, com os textos que exigem aviso.

## Fora do binário

| Item | Licença | Por que não afeta a licença do cade |
|---|---|---|
| Mage v1.17.2 | Apache-2.0 | dependência `tool`: compila o cade e nunca é ligado a ele (`internal/buildinfo` já o ignora) |
| Modelos (nomic-embed-text-v2-moe, Qwen3.5-2B e seu `mmproj`) | Apache-2.0 | dados baixados à parte por `go tool mage models`, nunca distribuídos com o cade |
| Qwen2.5-3B-Instruct (padrão antigo) | Qwen Research License, não comercial | não é mais baixado; uma configuração que aponte para ele roda sob essa licença |
| Runtime CUDA e cuBLAS da NVIDIA | proprietária | só o build CUDA local os liga; veja as exceções |

## Restrições e exceções

- **Builds CUDA não são distribuídos.** As releases são só para CPU. Nem a GPLv2 nem a GPLv3 tratam com clareza as bibliotecas CUDA como bibliotecas de sistema, então distribuir um binário CUDA exigiria antes uma exceção de ligação explícita do detentor dos direitos. Compilar localmente não tem problema: a GPL só impõe condições à distribuição.
- **Código GPL-2.0-only não pode mais entrar.** Ele não pode ser combinado com a GPLv3. Código "GPL-2.0-or-later" continua podendo.
- **sqlite-vec** continua sob MIT; a opção Apache-2.0 agora também serviria, mas não muda nada.
- **Os modelos são obras separadas** e mantêm suas licenças; um novo modelo padrão precisa ser conferido no card e no `general.license` do GGUF, como na fase 18.

## Conferindo uma dependência nova

Um módulo ou biblioteca ligado ao cade precisa ser compatível com `GPL-3.0-or-later`:

- **Sim:** MIT, MIT-0, BSD, ISC, zlib, Apache-2.0, MPL-2.0, domínio público / Unlicense / CC0, LGPL (2.1 ou posterior, 3), GPL-2.0-or-later, GPL-3.0. AGPL-3.0 também é permitida (GPLv3 §13), mas essa parte mantém sua cláusula de rede.
- **Não:** GPL-2.0-only, EPL-1.0, CDDL, SSPL, proprietária, termos não comerciais ou "Commons Clause".

Depois, acrescente-a ao `THIRD_PARTY_NOTICES.md` com o texto da licença; o `internal/buildinfo` falha quando o `go.mod` ganha um módulo que o arquivo não nomeia.
