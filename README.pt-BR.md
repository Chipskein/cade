# cade

[![CI](https://github.com/Chipskein/cade/actions/workflows/ci.yml/badge.svg?branch=dev)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Chipskein/cade/badges/coverage.json)](https://github.com/Chipskein/cade/actions/workflows/ci.yml)

[English](README.md) · **Português**

CLI de histórico pessoal. Ingere commits git, histórico do navegador, arquivos e mensagens do Teams, e permite consultar por data ou por pergunta em linguagem natural.

Todo o processamento é local: SQLite + sqlite-vec para armazenamento e busca vetorial, llama.cpp embutido para embeddings e geração. O que fica guardado, onde e como apagar: [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md).

Perguntas podem ser feitas em português ou inglês; a resposta vem no idioma da pergunta. `cade help` e `cade <comando> -h` seguem o idioma do sistema (`LC_ALL`, `LC_MESSAGES`, `LANG`: português para `pt*`, inglês nos outros casos); os demais rótulos da CLI são em português.

## Índice

- [Como funciona](#como-funciona)
- [Modelos](#modelos)
- [Build](#build)
- [Uso](#uso)
  - [Perguntas (`ask`)](#perguntas-ask)
- [Exemplo de saída](#exemplo-de-saída)
- [Tarefas](#tarefas)
- [Teams](#teams)
- [Configuração](#configuração)
- [Testes](#testes)
- [Privacidade](PRIVACY.pt-BR.md)
- [Benchmarks com gráficos](docs/BENCHMARKS.md)
- [Changelog](CHANGELOG.pt-BR.md): migrações e o que cada uma reescreve
- [Roadmap](docs/ROADMAP.md)

## Como funciona

```mermaid
flowchart LR
    subgraph Fontes
        git[Git]
        nav[Navegador]
        arq[Arquivos]
        teams[Teams · IndexedDB]
    end

    Fontes --> ingest[cade ingest<br/>normaliza + embedding]
    ingest --> db[(SQLite + sqlite-vec)]

    timeline[cade timeline] --> db
    tasks[cade tasks] --> relatorio[Tarefas e PRs<br/>por links de tarefa e PR]
    relatorio --> db

    ask[cade ask] --> plano[Interpreta a pergunta<br/>regras, ou LLM + gramática]
    plano -->|listar| filtro[Filtra no banco]
    plano -->|responder| busca[Filtra + busca vetorial]
    plano -->|tarefas| relatorio
    filtro --> db
    busca --> db
    busca --> llm[LLM local<br/>resposta com fontes]
```

## Modelos

| Uso | Modelo | Tamanho |
|---|---|---|
| Embeddings | [nomic-embed-text-v2-moe](https://huggingface.co/nomic-ai/nomic-embed-text-v2-moe-GGUF) Q4_K_M | 344 MB |
| Geração | [Qwen2.5-3B-Instruct](https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF) Q4_K_M | 2,1 GB |

Qualquer modelo GGUF compatível com llama.cpp pode ser usado via `embedding.model_path` e `generation.model_path` na configuração.

## Build

Requisitos: Go, gcc, cmake, ninja, curl.

```sh
make build    # compila llama.cpp e gera bin/cade
make models   # baixa os modelos para ~/.local/share/cade/models
make cuda     # opcional: build com GPU NVIDIA (requer CUDA Toolkit; gpu_layers -1 na configuração)
make install  # copia bin/cade para ~/.local/bin (PREFIX=... para mudar)
make uninstall
```

`uninstall` remove só o binário. Modelos, configuração e banco ficam em `~/.local/share/cade` e `~/.config/cade`.

Compile pelo `make`: a busca por palavra usa o FTS5 do SQLite, que o driver Go só compila com `-tags sqlite_fts5` (um `go build` puro gera um binário que se recusa a abrir o banco e diz por quê). Para `go test` no editor, use a mesma tag (VS Code: `"go.buildTags": "sqlite_fts5"`).

## Uso

```sh
cade init                                   # cria ~/.config/cade/config.json
cade ingest git ~/src/projeto
cade ingest all                             # alvos da configuração
cade timeline ontem
cade timeline --source git 2026-09-01 2026-09-07
cade ask "o que eu fiz relacionado a cache?"
cade ask --source teams --from 2026-09-01 "quando ficou marcado o deploy?"
cade ask --json "o que fiz sobre cache?"         # plano + resultado + origem de cada evento, em JSON
cade tasks ontem                            # tarefas trabalhadas e concluídas (PR aberto)
cade ask "quais tarefas finalizei essa semana?"   # mesmo relatório, em linguagem natural
cade forget teams                           # apaga os eventos de uma fonte, para reingerir
cade reindex                                # recalcula os vetores após trocar o modelo de embedding
cade teams-schema DIR                       # estrutura (sem valores) de um IndexedDB, para diagnóstico
```

Fontes: `git`, `browser`, `file`, `teams`. Flags vêm antes dos argumentos.

### Perguntas (`ask`)

No `ask`, o modelo local lê a pergunta e extrai só os filtros que ela afirma: período, fonte, pessoas, direção (recebidas/enviadas), assunto e se é sobre tarefas. O resultado aparece no stderr:

```
Entendi: listar · teams · 2026-09-25 · pessoas: Ana · recebidas
```

- Pedidos de lista com período ("as mensagens da Ana ontem") listam todos os eventos que casam, direto do banco.
- Perguntas com pessoa ou direção são respondidas só com os eventos que casam.
- Perguntas sobre tarefas ("quais tarefas finalizei ontem?", "o que ficou em andamento?") devolvem o relatório do `cade tasks`, opcionalmente só com as concluídas ou só com as em andamento; sem período, hoje. Com pessoa ou direção ("tarefas que a Ana me passou ontem"), só as tarefas com link nessas mensagens; um nome que não é de ninguém (um cliente) filtra pelo texto.
- Nomes são comparados por palavra inteira, ignorando maiúsculas, acentos, letras dobradas e y/i ("sillva" encontra "Leandro Silva"). Um nome que não é de nenhum remetente ou conversa (um cliente, um apelido) filtra pelo texto em vez de ser descartado.
- "Recebidas" deixa de fora mensagens de grupo que só marcam outras pessoas ("pronto? @Vitor"); uma menção a você, a um time ou tag mantém a mensagem.
- Mensagens sem conteúdo ("ok", "valeu", "bom dia") não entram como evidência nas respostas; as listagens continuam mostrando.
- A busca é híbrida: significado (vetores) e palavras (FTS5) são combinados. Uma pergunta que cita um identificador — código de tarefa ou de erro (`PROJ-481`, `ORA-01722`), hash de commit, número de PR — traz os eventos que o contêm.
- Repetições contam uma vez nas respostas: 12 visitas a uma página ou várias versões de uma nota viram uma fonte só, mostrada como "(12 visitas, última em …)". Arquivos apagados da pasta saem das respostas, mas continuam na timeline.
- Período, fonte, pessoas e direção são filtros exatos; só o assunto é buscado por significado ("commits de ontem sobre autenticação" busca "autenticação" entre os commits de ontem). Perguntas sem filtro são buscadas inteiras.
- Perguntas simples, feitas só de período, fonte e palavras genéricas ("liste os commits de ontem", "o que fiz hoje?", "quais tarefas finalizei hoje?"), são lidas por regras, sem o modelo; uma listagem ou relatório de tarefas lido assim nem carrega modelo. Uma pergunta com nome, assunto ou qualquer outra palavra vai para o modelo.
- As instruções e exemplos fixos do modelo são lidos uma vez, e o estado resultante fica salvo em `~/.cache/cade/prompt-state/` (~55 MB), então as perguntas seguintes pulam essa parte. O arquivo é refeito quando o modelo, o prompt ou o llama.cpp mudam.
- Flags (`--source`, `--from`, `--to`) têm prioridade; `--no-filters` desativa a interpretação.

## Exemplo de saída

```
$ cade timeline 2026-09-25
Timeline de 2026-09-25 — 5 eventos

── 2026-09-25 (Fri) ──
09:30  [git]     Corrige bug de timeout no login OAuth  (repo 79589eae)
11:20  [file]    ~/notas/reuniao.md
14:10  [browser] sqlite-vec: vector search SQLite extension — https://github.com/asg017/sqlite-vec
16:20  [browser] Redis client-side caching — https://redis.io/docs/latest/develop/use/client-side-caching/
16:45  [git]     Implementa cache Redis para sessões  (repo ebefa976)

$ cade ask "liste os commits de 25/09"
Entendi: listar · git · 2026-09-25
Timeline de 2026-09-25 — 2 eventos

── 2026-09-25 (Fri) ──
09:30  [git]     Corrige bug de timeout no login OAuth  (repo 79589eae)
16:45  [git]     Implementa cache Redis para sessões  (repo ebefa976)

$ cade ask "que páginas visitei em 25/09 sobre redis?"
Entendi: listar · browser · 2026-09-25 · assunto: redis
Timeline de 2026-09-25 — 1 eventos

── 2026-09-25 (Fri) ──
16:20  [browser] Redis client-side caching — https://redis.io/docs/latest/develop/use/client-side-caching/

$ cade ask "qual a receita de bolo de chocolate que eu vi?"
Entendi: responder · file · assunto: receita de bolo de chocolate
Não encontrei informação sobre isso nos dados ingeridos.
```

Pergunta com pessoa (nomes fictícios): só as mensagens com o Rui entram na busca.

```
$ cade ask "qual o problema com o CEP que comentei com o Rui?"
Entendi: responder · pessoas: Rui · assunto: problema com o CEP
Filtrando por pessoa: rui
O CEP cadastrado não existe mais e precisa ser atualizado [2]; o Rui perguntou se o cliente tinha alterado o endereço [1].

Fontes citadas:
  [1] [teams]   2026-09-25 09:30  Rui Costa: eles alteraram o CEP? ou precisa alterar para esse?  (chat Carla Dias, Rui Costa)
      ↳ https://teams.microsoft.com/l/message/19:a1b2c3@unq.gbl.spaces/1758803400000
  [2] [teams]   2026-09-25 09:31  Carla Dias: esse CEP que está cadastrado não existe mais  (chat Carla Dias, Rui Costa)
      ↳ https://teams.microsoft.com/l/message/19:a1b2c3@unq.gbl.spaces/1758803460000
```

Cada fonte citada mostra onde está o original (`↳`): `repositório@hash` para um commit, um link que abre a mensagem no Teams; páginas e arquivos já mostram a URL ou o caminho. Se a resposta citar um número que não corresponde a nenhum evento consultado, um aviso diz que aquele trecho não tem fonte.

`--json` imprime o plano resolvido e o resultado com uma referência (uid, fonte, data, localizador) de cada evento usado: as evidências dadas ao modelo e quais foram citadas, os eventos listados, ou cada tarefa com seus PRs e eventos.

## Tarefas

`cade tasks [--all] [DATA [FIM]]` (padrão: hoje), ou uma pergunta sobre tarefas no `cade ask`, lista as tarefas trabalhadas, só com eventos locais:

- **Tarefa:** link de um rastreador (proj4me, Jira, Linear, GitHub Issues, Azure Boards por padrão; qualquer regex em `tasks.task_url_patterns`) numa visita ou mensagem.
- **Concluída:** um PR aberto por você (GitHub, GitLab, Bitbucket, Azure DevOps) ligado a ela. Aberto por você = visita à página de criação do PR logo antes, ou mensagem sua com o link. Ligado = mensagem com os dois links, ou título do PR citando o id da tarefa (`fix-cep-162`, `PROJ-123 ...`); senão "(provável)" se aberto logo após trabalhar na tarefa.
- **Sua ou não:** a tarefa é sua se você abriu um PR para ela ou enviou uma mensagem citando-a; "consultada" se você só abriu a página; tarefas que só apareceram em mensagens de outras pessoas viram um resumo (`--all` lista).

```
$ cade tasks ontem
Tarefas de 2026-09-25 — 2 suas

concluída     14/162  Ajuste de CEP  (38 eventos)
              PR acme/api#45 aberto 16:40 · fix-cep-162

em andamento  14/170  Upload de arquivos  (21 eventos)

Consultadas (você abriu a tarefa; sem PR ou mensagem sua):

em andamento  14/171  Revisão de layout  (4 eventos)

Citadas só por outras pessoas: 3 tarefas — use --all para listar.
Sem tarefa: 12 eventos
```

## Teams

As mensagens são lidas do IndexedDB do Teams web no Chrome:

```sh
T=~/.config/google-chrome/Default/IndexedDB
cade ingest teams $T/https_teams.cloud.microsoft_0.indexeddb.leveldb \
                  $T/https_teams.microsoft.com_0.indexeddb.leveldb
```

Apenas mensagens já carregadas pelo cliente estão disponíveis.

Se uma atualização do Teams renomear o que o cade lê, o `ingest teams` falha com "formato do Teams não reconhecido" em vez de não achar nada em silêncio. As mensagens já guardadas não são afetadas. `cade teams-schema DIR` mostra a nova estrutura sem valores, para adaptar o leitor.

Eventos já ingeridos só são reprocessados se o conteúdo mudou na fonte (uma mensagem editada no Teams substitui o texto guardado; o `ingest` os conta como "atualizados"). Após atualizar o `cade`, para reingerir uma fonte do zero:

```sh
cade forget teams && cade ingest teams
```

O que já saiu da fonte (ex.: cache do Teams expirado) não volta.

## Configuração

`~/.config/cade/config.json`:

```json
{
  "sources": {
    "git_repositories": ["~/src/projeto"],
    "browser_histories": ["~/.config/google-chrome/Default/History"],
    "directories": ["~/notas"],
    "teams_indexeddb_dirs": ["~/.config/google-chrome/Default/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb"]
  }
}
```

Outros campos (criados pelo `cade init`):

| Campo | Padrão | Para quê |
|---|---|---|
| `generation.gpu_layers`, `embedding.gpu_layers` | `-1` | camadas na GPU (build CUDA); `-1` = todas |
| `generation.threads`, `embedding.threads` | `0` | threads de CPU; `0` = núcleos físicos |
| `retrieval.top_k` | `8` | eventos enviados ao modelo por pergunta |
| `retrieval.max_distance` | `0.72` | corte de relevância em perguntas sem filtros |
| `retrieval.mode` | `hybrid` | `hybrid` junta busca por significado e por palavra (FTS5); `vector` ou `lexical` usam uma só |
| `retrieval.max_best_distance` | `0.61` | pergunta sem filtro só é respondida se o evento mais próximo estiver a essa distância; aumente se perguntas reais derem "não encontrei" (`--verbose` registra a distância) |
| `retrieval.max_filtered_events` | `1000` | pergunta com pessoa ou direção ordena um a um até essa quantidade de eventos que casam; acima disso, busca no índice vetorial e fica com os resultados que casam. `0` = sem limite |
| `sources.git_authors` | `[]` | ingere só commits desses autores |
| `sources.git_identities` | `["auto"]` | seus e-mails ou nomes de commit; `auto` lê `git config user.email`/`user.name` de cada repositório. Commits de outras pessoas ficam no banco, mas saem da `timeline` (veja `--all-authors`), das perguntas em primeira pessoa ("o que eu fiz?") e do relatório de tarefas |

Notas, mensagens e commits longos são divididos em pedaços de até ~1.200 caracteres (o modelo de embedding lê 512 tokens), e a resposta mostra o pedaço que casou ("arquitetura.md, trecho 7 de 20"). Ao atualizar de uma versão sem pedaços, rode `cade reindex` uma vez: ele embute os eventos longos (1.568 de 108 mil num histórico real, cerca de um minuto).

Para trocar o modelo de embedding, ajuste `embedding.model_path` e rode `cade reindex`: ele recalcula todos os vetores a partir do texto guardado e continua de onde parou se for interrompido (~300 eventos/s numa RTX 3060, alguns minutos para 100 mil eventos). O banco registra de qual modelo vieram os vetores; `ingest` e `ask` recusam um modelo diferente em vez de misturar vetores incompatíveis.

Mudanças de esquema são aplicadas automaticamente ao abrir o banco (migrações numeradas). Um passo que reescreve dados antes salva uma cópia como `cade.db.before-vN-<data>` e avisa onde; apague-a quando estiver satisfeito.

## Testes

```sh
make test
make test-models   # inclui testes com os modelos reais
make eval-plan     # mede a interpretação das perguntas (GPU se o CUDA Toolkit estiver instalado; GO_TAGS= força CPU)
make eval-retrieval  # mede a busca: recall, MRR, rejeição
make eval          # as duas
make bench         # latência e memória: banco com 1k/10k/100k eventos, modelos, um ask inteiro (GO_TAGS= para o build CPU)
make check         # o que a CI roda: gofmt, go vet, golangci-lint, testes
make cover         # testes com cobertura (por função, total no fim)
```

**CI** (GitHub Actions, `.github/workflows/`):
- `ci.yml`, a cada push e pull request: `make fmt-check`, `vet`, `lint` e `cover`. O build do llama.cpp fica em cache pela `LLAMA_TAG`, compilado com `LLAMA_NATIVE=OFF` (AVX2, sem ajuste à CPU do runner) para que a biblioteca em cache rode em qualquer runner. O total de cobertura vai para o resumo da execução e, no `dev`, para o badge acima (um `coverage.json` no branch `badges`, sem serviço externo).
- `eval.yml`, manual ou toda segunda-feira: `make eval` em CPU com os modelos em cache, e o relatório publicado como o artefato `eval-report`. Leva horas num runner, por isso fica fora do caminho de cada push.

O `golangci-lint` roda `errcheck`, `staticcheck`, `unused` e `ineffassign` (`.golangci.yml`); instale a versão fixada no Makefile (`make -s print-GOLANGCI_LINT_VERSION`) com `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<versão>`.

`eval-plan` passa ~150 perguntas de `testdata/queries/plan.json` (período, git, Teams, navegador, arquivos, busca semântica, pessoas, tarefas, empresas lidas como pessoa; PT e EN) pelo modelo real e mostra a taxa de acerto de cada campo (tipo, período, fonte, pessoas, direção, assunto, status) com o intervalo de Wilson de 95%, e cada pergunta interpretada errado. Falha quando o limite inferior de um campo cai abaixo do `minimum_accuracy` do arquivo, então mudanças no prompt ou no modelo não pioram a interpretação em silêncio, e um caso isolado não reprova. `testdata/README.md` explica como transformar uma pergunta real num caso anonimizado.

`eval-retrieval` ingere um corpus sintético (`testdata/queries/retrieval/corpus.json`: ~290 commits, páginas, arquivos e mensagens, com parecidos como PROJ-418 ao lado de PROJ-481, páginas visitadas muitas vezes, um arquivo em várias versões, notas longas com a resposta no fim, commits de outros autores e conversa do dia a dia) num SQLite real com o embedder real. As perguntas vêm em dois conjuntos: `calibration.json` mostra onde os limites de distância deveriam ficar (sem mudá-los), e `test.json`, nunca usado para ajustar, é conferido contra os pisos. Mede recall e MRR nas perguntas com resposta, rejeição (perguntas que nada responde não devem trazer nada) e redundância (resultados que repetem a mesma página ou arquivo). `make eval-scale SCALE=1000,10000` roda o teste em corpora aumentados com distratores e salva a curva em `bench/retrieval-scale.txt`; `bench/retrieval-baseline.txt` guarda os resultados antes das mudanças de busca da próxima versão.

`bench` mede o banco em históricos sintéticos de 1 mil, 10 mil e 100 mil eventos (busca vetorial, leitura por período, gravação, bytes por evento) e os modelos (embedding de um evento, interpretação da pergunta, geração da resposta), com a memória do processo e da GPU. O `BenchmarkColdAsk` cronometra um `cade ask` inteiro até o primeiro token da resposta (com o carregamento dos dois modelos), com o cache de página quente ou esvaziado, e a pergunta lida pelo modelo, pelo modelo com o estado do prompt salvo, ou por regras. `bench/baseline.txt` tem uma execução de referência numa RTX 3060, e `bench/baseline-cpu.txt`, a mesma máquina sem a GPU; salve as novas execuções e compare com o [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).
