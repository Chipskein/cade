<p align="center">
  <img src="../assets/cade.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Test data

**English** · Português abaixo

| Path | What it is |
|---|---|
| `queries/plan.json` | Questions and the plan the interpreter must produce (`go tool mage evalPlan`). |
| `queries/retrieval/corpus.json` | Synthetic history searched by the retrieval suite. |
| `queries/retrieval/calibration.json` | Retrieval questions used to tune thresholds. Report only, no floors. |
| `queries/retrieval/test.json` | Retrieval questions never used for tuning, with floors (`go tool mage evalRetrieval`). |
| `chrome-indexeddb.leveldb`, `chrome-indexeddb-pages/` | Synthetic Chrome IndexedDB for the IndexedDB and LevelDB readers, written by a real Chrome from the pages. |
| `teams-formats/` | One Teams cache per format seen, synthetic, written the same way; the Teams reader must keep recognizing each. |
| `queries/captions.json` | Words each image fixture's description must contain, and secrets it must not keep once masked (`go tool mage evalCaptions`). |
| `queries/injection.json` | Questions whose evidence includes an event, or an image, that gives the model orders (`go tool mage evalInjection`). |
| `images/` | Image fixtures of phase 19. The screenshots and the diagram are drawn by `go run testdata/images/generate.go` with invented text (the token and the key only have the shape of real ones). `photo-earth.jpg` is NASA's public-domain photo [The Earth seen from Apollo 17](https://commons.wikimedia.org/wiki/File:The_Earth_seen_from_Apollo_17.jpg), scaled to 960 px. |

## Turning a real question into a case

Real questions are the best cases: they show what actually goes wrong. But this repository is public, and your history holds other people's names, clients and messages. **Nothing real goes in. Keep the shape of the problem and change everything else.**

1. **Write down the problem, not the data.** For example: "a client name was read as a person", "a group message that @mentions someone else was listed as received", "a misspelled name found nobody".
2. **Replace every identifying detail:**
   - **People:** use fictional names already in the suites (Ana Souza, Carla Dias, Rui Costa, Marcos Lima, Juliana Prado, Pedro Alves, Beatriz Nunes, Leandro Silva, Sofia, Tiago, Vitor Alves).
   - **Companies, clients, projects:** Acme, Zenite, Globex, Orion, Atlas, Hermes, Norteagro, Solaris.
   - **URLs:** keep public documentation as is; use `example.com` for anything internal.
   - **Task IDs, hashes, PR numbers:** change the digits, but keep the format (`PROJ-481`, a 10-character hex hash).
3. **Paraphrase message text.** Never paste it, even with names changed.
4. **Keep what triggers the bug:** the misspelling pattern (doubled letters, y/i), the company in the person's slot, the mention of another person, the answer buried at the end of a long note.
5. **Choose the set:**
   - Retrieval cases go in `test.json` when they should hold as a regression, or in `calibration.json` when they will guide a threshold.
   - Do not move a case from the test set to calibration after seeing it fail: that turns the test into tuning data.
6. **Check before committing:**
   - `go tool mage test` validates the files, including that plan dates match the date parser.
   - `go tool mage eval` shows the new case failing before the fix and passing after it.
   - Search the diff for real names (`git diff | grep -i <name>`).

---

## Adding a Teams cache format

When a Teams update changes its cache and `ingest teams` reports an unrecognized format, the reader is adapted and the new format kept as a regression sample, without any real message:

1. Run `cade teams-schema DIR` on your cache: it prints store and field names with types and counts, no values.
2. Write a page in `chrome-indexeddb-pages/` that creates the same databases, stores and fields with synthetic content and the fictional names above (see `teams-2026-09.html`).
3. Generate the sample with a real Chrome: `./chrome-indexeddb-pages/generate.sh ../teams-formats/<yyyy-mm>.leveldb <page>.html` (needs `google-chrome-stable`).
4. Add an entry to `formatSamples` in `internal/ingest/teamssource/format_sample_test.go` with the messages the reader must produce. Keep the old samples: users may still have caches in those formats.

## Português

Perguntas reais são os melhores casos, porque mostram o que dá errado de verdade. Mas o repositório é público, e o seu histórico tem nomes de outras pessoas, clientes e mensagens. **Nada real entra. Mantenha a forma do problema e troque todo o resto.**

1. **Descreva o problema, não os dados.** Por exemplo: "nome de cliente lido como pessoa", "mensagem de grupo que marca outra pessoa listada como recebida", "nome escrito errado não achou ninguém".
2. **Troque todo detalhe que identifique alguém:**
   - **Pessoas:** use os nomes fictícios que já estão nas suítes (Ana Souza, Carla Dias, Rui Costa, Marcos Lima, Juliana Prado, Pedro Alves, Beatriz Nunes, Leandro Silva, Sofia, Tiago, Vitor Alves).
   - **Empresas, clientes, projetos:** Acme, Zenite, Globex, Orion, Atlas, Hermes, Norteagro, Solaris.
   - **URLs:** documentação pública pode ficar; o que for interno vira `example.com`.
   - **IDs de tarefa, hashes, números de PR:** troque os dígitos, mas mantenha o formato (`PROJ-481`, hash hexadecimal de 10 caracteres).
3. **Reescreva o texto das mensagens com outras palavras.** Nunca cole o original, nem com os nomes trocados.
4. **Mantenha o que dispara o problema:** o padrão do erro de grafia (letra dobrada, y/i), a empresa no lugar da pessoa, a menção a outra pessoa, a resposta escondida no fim de uma nota longa.
5. **Escolha o conjunto:**
   - Casos de busca vão para `test.json` quando devem valer como regressão, ou para `calibration.json` quando vão orientar um limite.
   - Não mova um caso do teste para a calibração depois de vê-lo falhar: isso transforma o teste em dado de ajuste.
6. **Confira antes do commit:**
   - `go tool mage test` valida os arquivos, inclusive se as datas do `plan.json` batem com o parser de datas.
   - `go tool mage eval` mostra o caso novo falhando antes da correção e passando depois.
   - Procure nomes reais no diff (`git diff | grep -i <nome>`).

## Imagens

`images/` guarda as imagens da fase 19. As capturas e o diagrama saem de `go run testdata/images/generate.go`, com texto inventado (o token e a chave só têm o formato de reais). `photo-earth.jpg` é a foto da NASA em domínio público [The Earth seen from Apollo 17](https://commons.wikimedia.org/wiki/File:The_Earth_seen_from_Apollo_17.jpg), reduzida a 960 px. O `queries/captions.json` diz que palavras a descrição de cada uma precisa ter e que segredos não podem sobrar depois da máscara (`go tool mage evalCaptions`); uma imagem nova precisa de um caso lá (um teste confere).

## Um novo formato do cache do Teams

Quando uma atualização do Teams muda o cache e o `ingest teams` acusa formato não reconhecido, o leitor é adaptado e o novo formato fica como amostra de regressão, sem nenhuma mensagem real:

1. Rode `cade teams-schema DIR` no seu cache: ele mostra nomes de stores e campos, com tipos e contagens, sem valores.
2. Escreva uma página em `chrome-indexeddb-pages/` que crie os mesmos bancos, stores e campos com conteúdo sintético e os nomes fictícios acima (veja `teams-2026-09.html`).
3. Gere a amostra com um Chrome de verdade: `./chrome-indexeddb-pages/generate.sh ../teams-formats/<aaaa-mm>.leveldb <página>.html` (requer `google-chrome-stable`).
4. Acrescente uma entrada em `formatSamples`, em `internal/ingest/teamssource/format_sample_test.go`, com as mensagens que o leitor deve produzir. Mantenha as amostras antigas: pode haver caches nesses formatos.
