# cade

[English](README.md) · **Português**

CLI de histórico pessoal. Ingere commits git, histórico do navegador, arquivos e mensagens do Teams, e permite consultar por data ou por pergunta em linguagem natural.

Todo o processamento é local: SQLite + sqlite-vec para armazenamento e busca vetorial, llama.cpp embutido para embeddings e geração. O que fica guardado, onde e como apagar: [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md).

Perguntas podem ser feitas em português ou inglês; a resposta vem no idioma da pergunta.

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

    ask[cade ask] --> plano[Interpreta a pergunta<br/>LLM + gramática]
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
- Nomes são comparados por palavra inteira, ignorando maiúsculas, acentos, letras dobradas e y/i ("avilla" encontra "Leandro Avila"). Um nome que não é de nenhum remetente ou conversa (um cliente, um apelido) filtra pelo texto em vez de ser descartado.
- "Recebidas" deixa de fora mensagens de grupo que só marcam outras pessoas ("pronto? @Vitor"); uma menção a você, a um time ou tag mantém a mensagem.
- Mensagens sem conteúdo ("ok", "valeu", "bom dia") não entram como evidência nas respostas; as listagens continuam mostrando.
- Período, fonte, pessoas e direção são filtros exatos; só o assunto é buscado por significado ("commits de ontem sobre autenticação" busca "autenticação" entre os commits de ontem). Perguntas sem filtro são buscadas inteiras.
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
| `retrieval.max_best_distance` | `0.62` | pergunta sem filtro só é respondida se o evento mais próximo estiver a essa distância; aumente se perguntas reais derem "não encontrei" (`--verbose` registra a distância) |
| `sources.git_authors` | `[]` | ingere só commits desses autores |

Trocar o modelo de embedding exige um banco novo.

## Testes

```sh
make test
make test-models   # inclui testes com os modelos reais
make eval-plan     # mede a interpretação das perguntas (GPU se o CUDA Toolkit estiver instalado; GO_TAGS= força CPU)
make eval-retrieval  # mede a busca: recall, MRR, rejeição
make eval          # as duas
make bench         # latência e memória: banco com 1k/10k/100k eventos, modelos
```

`eval-plan` passa as perguntas de `testdata/queries/plan.json` (período, git, Teams, navegador, arquivos, busca semântica, tarefas, PT e EN) pelo modelo real e mostra a taxa de acerto de cada campo (tipo, período, fonte, pessoas, direção, assunto, status) e cada pergunta interpretada errado. Falha quando um campo cai abaixo do `minimum_accuracy` do arquivo, então mudanças no prompt ou no modelo não pioram a interpretação em silêncio.

`eval-retrieval` ingere um corpus sintético (`testdata/queries/retrieval.json`: ~250 commits, páginas, arquivos e mensagens, com parecidos como PROJ-418 ao lado de PROJ-481 e conversa do dia a dia) num SQLite real com o embedder real, e confere se cada pergunta traz os eventos que a respondem. Mede recall e MRR nas perguntas com resposta, e rejeição: perguntas que nada responde não devem trazer nada.

`bench` mede o banco em históricos sintéticos de 1 mil, 10 mil e 100 mil eventos (busca vetorial, leitura por período, gravação, bytes por evento) e os modelos (embedding de um evento, interpretação da pergunta, geração da resposta), com a memória do processo e da GPU. `bench/baseline.txt` tem uma execução de referência numa RTX 3060; salve as novas execuções e compare com o [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).
