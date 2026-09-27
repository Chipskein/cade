# Changelog

[English](CHANGELOG.md) · **Português**

O que mudou em cada versão, as migrações de esquema e o que cada uma reescreve. O que falta para a release está em [docs/ROADMAP.md](docs/ROADMAP.md). Os gráficos estão em [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

## Não lançado (primeira versão)

### Atualizar um banco existente

- **Quando:** as migrações rodam sozinhas na primeira vez que um comando abre o banco.
- **Cópia:** antes da primeira migração que reescreve dados, o cade grava uma cópia, `cade.db.before-vN-<data>`, com permissão `600`. Pode apagá-la depois de conferir que tudo funciona.
  - Numa cópia real (108 mil eventos, esquema 2 → 6), a migração levou ~50 s, e o banco foi de 516 MB para 450 MB.
- **Reindexação:** depois da migração 5, rode `cade reindex` uma vez. Até ele terminar, os textos longos ficam sem vetor, e o `ask` avisa.
  - Na mesma cópia, levou ~80 s numa RTX 3060.
- **Sem reimportar:** reimportar perderia dados, porque o cache do Teams expira e o Chrome guarda só ~90 dias de histórico.

### Migrações

| Versão | O que faz | Cópia | Reescreve dados |
|---|---|---|---|
| 1 | tabelas `events` e `store_settings` | — | — |
| 2 | guarda o texto das mensagens do Teams no metadado | sim | eventos do Teams |
| 3 | `content_hash`, para reaproveitar o vetor de textos iguais | não | acrescenta uma coluna |
| 4 | um evento por arquivo; versões anteriores viram datas e tamanhos em `file_modifications` | sim | eventos de arquivo (versões antigas juntadas, UIDs reescritos) |
| 5 | vetores por pedaço (`chunks`, `chunk_embeddings`) em vez de por evento; o banco é compactado no fim | sim | vetores (eventos longos esperam o `cade reindex`) |
| 6 | índice de palavras sobre os pedaços (`chunks_fts`, FTS5) | não | preenche o índice com os pedaços existentes |
| 7 | índice de pessoas (`event_people`) e direção das mensagens (`events.direction`) | não | preenche os dois a partir dos eventos existentes |

O binário precisa ser compilado com a tag `sqlite_fts5`, e o `make` já faz isso. Sem ela, abrir o banco falha com uma mensagem clara.

### Filtros de pessoa no SQL (fase 6)

- **Problema:** uma pergunta com pessoa e sem período ("o que a Ana me mandou?") carregava todos os eventos, com conteúdo, e filtrava em Go. A memória crescia com o banco.
- **Índice de pessoas:** a tabela `event_people` guarda, por evento, o remetente (ou o autor do commit), a conversa e os primeiros nomes @mencionados, já normalizados como a comparação de nomes faz (palavras inteiras, sem acento, letras dobradas e y/i juntadas). A coluna `events.direction` guarda se a mensagem foi enviada, recebida ou publicada num canal. Os dois são gravados na ingestão, e a migração 7 os preenche nos eventos existentes, sem reimportar. Numa cópia real (108 mil eventos), ela levou ~1,4 s e o banco cresceu 7 MB.
- **Filtro em SQL:** período, fonte, direção e pessoas viram uma condição sobre o índice. Cada nome é resolvido (nome inteiro, depois o primeiro nome) com uma contagem que para no primeiro evento, e só os eventos que casam são lidos. A regra de "recebida" (mensagem de grupo que só menciona outras pessoas conhecidas não conta) roda no mesmo SQL.
- **Limite:** acima de `retrieval.max_filtered_events` (1000) eventos que casam, como em "mensagens que recebi" sem período, a busca vai ao índice vetorial do período e da fonte, com `k` crescendo até 4096, e fica com os vizinhos que casam. Ordenar um evento lê os vetores dele (~0,19 ms), então 1000 ficam abaixo de 0,2 s.
- **Mesmo resultado:** um teste compara o filtro em SQL com o filtro em memória em 120 combinações (4 escopos × 3 direções × 10 conjuntos de nomes, com menções, canais, commits e variações de grafia). A suíte de recuperação dá resultado idêntico caso a caso (recall 1,00, MRR 0,89, rejeição 1,00).
- **Medido** (`BenchmarkPersonFilter`, pessoa sem período, 100 mil eventos sintéticos):

  | | antes | depois |
  |---|---|---|
  | tempo | 471 ms | 8 ms |
  | memória alocada | 187 MB | 0,7 MB |

  Com 1 mil e 10 mil eventos, antes alocava 1,5 MB e 17 MB; depois, 33 KB e 69 KB. Depois do filtro, a memória acompanha os eventos da pessoa, não o banco, e o limite a restringe. "Mensagens que recebi" sem período (`BenchmarkDirectionFilter`) aloca ~190 KB em qualquer tamanho.
- **Privacidade:** o `forget` apaga as linhas de `event_people` junto com os eventos (teste de privacidade).

### Integração contínua (fase 10)

- **A cada push e pull request** (`.github/workflows/ci.yml`): `gofmt`, `go vet`, `golangci-lint` e os testes com cobertura, todos com a tag `sqlite_fts5`. `make check` roda o mesmo localmente.
  - O build do llama.cpp fica em cache pela `LLAMA_TAG`. Ele é compilado com a nova opção `LLAMA_NATIVE=OFF` (AVX2, FMA, F16C), porque uma biblioteca ajustada à CPU de um runner pode falhar em outro. O build local continua `ON`.
- **Suítes com modelo fora do caminho de cada push** (`.github/workflows/eval.yml`): `make eval` em CPU, manual ou semanal, com os modelos e os embeddings do corpus em cache. O relatório é publicado como o artefato `eval-report`. `EVAL_TIMEOUT` (padrão `1h`) sobe o limite de 10 minutos do Go, que um runner em CPU ultrapassa.
- **Lint:** `errcheck`, `staticcheck`, `unused` e `ineffassign`, com a versão fixada no Makefile. O que eles acharam foi corrigido, não silenciado:
  - 75 linhas de teste ignoravam erros de passos de preparação (gravar fixtures, salvar eventos, rodar o pipeline), então uma preparação quebrada podia passar em silêncio ou falhar numa asserção posterior, enganosa. Agora elas param o teste (`internal/testcheck`).
  - dois testes rodavam uma chamada que falha de propósito sem conferir que ela falhou; agora verificam o erro.
  - as exclusões são o conjunto padrão do golangci (`Close`, impressões no terminal, remoção de arquivos temporários), mais `tx.Rollback` depois do `Commit`, cada uma com o motivo em `.golangci.yml`.
- **Cobertura:** 82,1% das instruções. O total vai para o resumo da execução e para um badge no README, servido por um `coverage.json` no branch `badges`, sem serviço externo.

### `ask` mais rápido (fase 5)

- **Regras antes do modelo:** uma pergunta feita só de período, fonte e palavras genéricas ("liste os commits de ontem", "o que fiz hoje?", "which tasks did I finish today?") é lida sem o modelo. Qualquer outra palavra (um nome, um assunto, um número) a manda para o modelo, então as regras nunca chutam.
  - Elas leem 67 das 153 perguntas da suíte de plano, todas certas. A suíte foi de 129 para 131 perguntas totalmente corretas, porque nessas o modelo às vezes inventava um assunto.
  - Uma listagem ou relatório de tarefas lido pelas regras nem carrega modelo; o gerador só é carregado quando algo precisa dele.
- **Estado do prompt salvo:** as instruções e exemplos fixos do planejador (~2 mil tokens) eram decodificados de novo a cada `cade ask`. Agora o estado deles é salvo uma vez em `~/.cache/cade/prompt-state/` (~55 MB, só o dono lê) e carregado nas execuções seguintes.
  - A chave cobre a versão e o commit do llama.cpp, o build (CPU ou CUDA), o arquivo do modelo (caminho, tamanho, data de modificação), o tamanho do contexto, as camadas na GPU e os tokens do prompt. Qualquer mudança grava um arquivo novo e apaga o antigo.
  - Não contém pergunta nenhuma nem nada do banco. Pode ser apagado; é refeito na próxima pergunta.
- **Medido** (`BenchmarkColdAsk`, até o primeiro token da resposta, com o carregamento dos dois modelos, cache de página quente):

  | | modelo, prompt inteiro | modelo, estado salvo | regras |
  |---|---|---|---|
  | CPU (Ryzen 5 5500) | 41,5 s | 26,0 s | 21,5 s |
  | GPU (RTX 3060) | 2,93 s | 2,48 s | 1,59 s |

  Com o cache de página frio (modelos tirados da memória, como depois de reiniciar), a CPU vai de 45,2 → 31,2 → 26,6 s, e a GPU de 8,8 → 8,5 → 7,7 s. O que sobra em CPU é quase todo o modelo lendo as evidências antes de responder.
- **Benchmarks:** o `make bench` também cronometra um `ask` inteiro, e `make bench GO_TAGS=` grava o baseline só de CPU em `bench/baseline-cpu.txt`.

### Autoria no git (fase 4)

- **Identidades:** cada commit é marcado como `mine` ou `other` por `sources.git_identities`.
  - O padrão, `["auto"]`, lê `git config user.email` e `user.name` em cada repositório.
  - Um commit sem marca conta como seu.
  - Os commits já gravados são marcados de novo a cada `ingest git`, sem migração.
- **Onde a marca vale:**
  - a `timeline` esconde commits de outros, e `--all-authors` mostra todos;
  - perguntas em primeira pessoa sem pessoa citada ("o que eu fiz…", "what did I do…") deixam de fora commits de outros;
  - o relatório de tarefas ignora commits de outros, e um commit seu que cita uma tarefa a torna sua.
- **Efeito:** num histórico real, 1.037 de 61.645 commits eram do usuário. Até agora, todos contavam como trabalho dele.

### Busca híbrida (fase 3)

- **Palavras:** a busca por palavras (FTS5, acentos ignorados) passa a rodar junto com a vetorial, e as duas ordenações são fundidas por rank recíproco.
- **Identificadores** (códigos de tarefa ou de erro como `PROJ-481`, hashes de commit, números de PR) viram filtro forte.
- **Modos:** `retrieval.mode` escolhe `hybrid` (padrão), `vector` ou `lexical`.
- **Resultados:**

  | | recall | MRR |
  |---|---|---|
  | conjunto de teste, antes | 0,87 | 0,81 |
  | conjunto de teste, depois | 1,00 | 0,88 |
  | 10 mil eventos, antes | 0,80 | 0,80 |
  | 10 mil eventos, depois | 0,93 | 0,82 |

  A rejeição ficou em 1,00.

### Pedaços (fase 2)

- **Divisão:** textos acima de 1.200 caracteres são divididos por títulos Markdown, parágrafos, linhas e espaços, com sobreposição de 120 caracteres. Cada pedaço tem o próprio vetor.
- **Na resposta:** o prompt recebe o pedaço que casou ("trecho i de n"), não o início do arquivo. O `--json` mostra `chunk`, `chunks`, `excerpt_start` e `excerpt_end`.
- **Resultados:**

  | | recall | MRR |
  |---|---|---|
  | conjunto de teste, antes | 0,80 | 0,74 |
  | conjunto de teste, depois | 0,87 | 0,81 |

  O ganho vem de uma nota longa com a resposta perto do fim.

### Deduplicação (fase 1)

- **Visitas repetidas** a uma página, ou trechos do mesmo arquivo, aparecem como um item, com a contagem e a data mais recente. A timeline continua mostrando cada visita.
- **Vetores:** um texto idêntico é embutido uma vez só. Num histórico real, 89% das visitas do navegador repetem um texto já gravado.
- **Arquivos:**
  - um evento por arquivo, com as datas de edição em `file_modifications`;
  - arquivos que somem da pasta saem das respostas;
  - dez versões de um arquivo de 200 KB ocupam o espaço de uma.
- **Resultados:** a redundância no conjunto de teste foi de 0,17 para 0,00.

### Corrigido

- **Contexto do embedder (fase 0.5):** o embedder rodava com contexto de 2.048 tokens, mas o modelo foi treinado com 512.
  - Agora usa o menor dos dois, e o `--verbose` mostra o contexto efetivo e cada corte.
  - Os vetores de textos longos só melhoram depois de um `cade reindex`.
- **Migrações:** abrir o banco com várias migrações pendentes fazia uma cópia por migração; agora faz uma só.
- **`forget`:** agora apaga também o histórico de arquivos e o índice de palavras.

### Avaliação (fase 0)

- **Suíte de recuperação:**
  - corpus de 291 eventos;
  - conjuntos separados de calibração (25 perguntas) e de teste (24 perguntas), e os limiares vêm só da calibração;
  - curva de escala com distratores sintéticos (`make eval-scale`);
  - métricas de recall, MRR, rejeição e redundância.
- **Suíte de plano:** 153 perguntas, com intervalos de Wilson de 95%. Os pisos são comparados com o limite inferior.
- **Anonimização:** o `testdata/README.md` explica como transformar uma pergunta real num caso de teste.
- **Benchmarks:** os modelos aquecem antes da medição.

### Antes, nesta versão

- **Perguntas:** o `cade ask` interpreta cada pergunta (tipo, fonte, período, pessoas, direção, assunto, status de tarefa). Os filtros rodam em SQL, e só o assunto é buscado por significado.
  - Perguntas sem filtro cujo evento mais próximo está longe são recusadas em vez de respondidas.
  - As respostas citam as fontes.
- **Tarefas:** o `cade tasks` mostra as tarefas do período, seus pull requests e se cada uma é sua.
- **Teams:**
  - o texto das mensagens é guardado (migração 2);
  - mensagens editadas substituem a versão anterior;
  - a ingestão falha com clareza num formato de cache desconhecido, e o `teams-schema` ajuda a diagnosticar.
- **Reindexação:** o `cade reindex` recalcula os vetores, e a troca do modelo de embedding é detectada.
- **Privacidade:**
  - só você lê o arquivo do banco e a pasta dele;
  - o texto substituído ou esquecido é zerado no disco;
  - veja [PRIVACY.pt-BR.md](PRIVACY.pt-BR.md).
- **Ajuda:** `cade help` e `-h` seguem o idioma do sistema (inglês ou português).
- **Licença:** GPLv2.
