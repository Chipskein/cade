# Test data

**English** · Português abaixo

| Path | What it is |
|---|---|
| `queries/plan.json` | Questions and the plan the interpreter must produce (`make eval-plan`). |
| `queries/retrieval/corpus.json` | Synthetic history searched by the retrieval suite. |
| `queries/retrieval/calibration.json` | Retrieval questions used to tune thresholds. Report only, no floors. |
| `queries/retrieval/test.json` | Retrieval questions never used for tuning, with floors (`make eval-retrieval`). |
| `chrome-indexeddb.leveldb`, `chrome-indexeddb-pages/` | Synthetic Chrome IndexedDB for the Teams reader. |

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
   - `make test` validates the files, including that plan dates match the date parser.
   - `make eval` shows the new case failing before the fix and passing after it.
   - Search the diff for real names (`git diff | grep -i <name>`).

---

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
   - `make test` valida os arquivos, inclusive se as datas do `plan.json` batem com o parser de datas.
   - `make eval` mostra o caso novo falhando antes da correção e passando depois.
   - Procure nomes reais no diff (`git diff | grep -i <nome>`).
