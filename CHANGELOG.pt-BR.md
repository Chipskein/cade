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

O binário precisa ser compilado com a tag `sqlite_fts5`, e o `make` já faz isso. Sem ela, abrir o banco falha com uma mensagem clara.

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
