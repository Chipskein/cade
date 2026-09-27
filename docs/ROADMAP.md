# cade — roadmap

O que falta para a primeira versão, e em que ordem. O que já foi entregue, com as medições de cada fase, está no [CHANGELOG](../CHANGELOG.pt-BR.md); os gráficos, em [BENCHMARKS.md](BENCHMARKS.md). Os requisitos citados no código (`RF`, `RNF`, `CA`) estão em [USECASES.md](USECASES.md).

## Como cada entrega é feita

- Uma fase (ou parte dela) por vez, cada uma num commit próprio.
- Cada entrega vem com testes (`make test`), a suíte (`make eval`) quando afeta busca ou plano, `make bench` quando afeta desempenho, e docs (README EN/PT, `docs/USECASES.md`, PRIVACY quando toca dados, CHANGELOG).
- Mudança de esquema é migração (`PRAGMA user_version`, com cópia quando reescreve dados), nunca `forget` + `ingest`: reimportar perde dados, porque o cache do Teams expira e o histórico do Chrome guarda só ~90 dias.
- Tudo continua local, sem rede em tempo de execução.

## Situação

| Fase | Tema | Situação | Impacto | Esforço |
| ---- | ---- | -------- | ------- | ------- |
| 0 | Avaliação confiável | concluída | pré-requisito | médio |
| 0.5 | Contexto do embedder (bug) | concluída | alto | baixo |
| 1 | Deduplicação de eventos | concluída | alto | médio |
| 2 | Chunking de arquivos | concluída | alto | médio |
| 3 | Busca híbrida (FTS5 + vetor) | concluída | alto | médio |
| 4 | Autoria no git | concluída | alto | baixo |
| 5 | Latência do `ask` | concluída | médio | médio |
| 10 | [Integração contínua e qualidade](#fase-10--integração-contínua-e-qualidade) | concluída; falta medir no GitHub | alto | baixo |
| 6 | [Filtros de pessoa no SQL](#fase-6--filtros-de-pessoa-no-sql) | pendente | médio | baixo |
| 7 | [Evidência não confiável no prompt](#fase-7--evidência-não-confiável-no-prompt) | pendente | baixo | baixo |
| 11 | [Instalação e configuração](#fase-11--instalação-e-configuração) | pendente | alto | médio |
| 12 | [Idioma da interface](#fase-12--idioma-da-interface) | pendente | médio | médio |
| 8 | [Documentação e manutenção](#fase-8--documentação-e-manutenção) | pendente | baixo | baixo |
| 9 | [Empacotamento da versão](#fase-9--empacotamento-da-versão) | pendente | pré-requisito do lançamento | baixo |
| — | [A definir](#a-definir) | em aberto | — | — |

A tabela está na ordem sugerida:
- **CI (10) primeiro:** é barata e protege todas as fases seguintes. Entregue; falta medir no GitHub.
- **Depois as mudanças de código:** 6 e 7. A 5 foi feita antes da 10, a pedido.
- **Em seguida, a experiência de quem instala:** 11 e 12.
- **Docs (8) e empacotamento (9) por último:** descrevem o estado final.

---

## Fase 10 — Integração contínua e qualidade

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)). Falta o que só dá para verificar com os workflows rodando no GitHub:

- registrar aqui o tempo do workflow de testes com o cache quente (a segunda execução em diante) e com o cache frio;
- confirmar que um PR que quebra um teste ou o `gofmt` fica vermelho;
- rodar o `eval.yml` uma vez à mão e registrar quanto leva em CPU.

---

## Fase 6 — Filtros de pessoa no SQL

### Problema

Em `internal/rag/restricted.go` (`candidates`), uma pergunta com pessoa mas sem período carrega **todos** os eventos, com conteúdo, desde 1970, e filtra em Go. O comentário assume que "os eventos de uma pessoa num período são poucos", o que não vale sem período. Ler tudo leva 345 ms em 100 mil eventos, e a memória cresce com o banco.

### Mudanças

1. **Índice de pessoas.**
   - Nova tabela `event_people(event_id, name_norm, role)`, com `role` entre sender, recipient, author e mentioned.
   - Nova coluna `direction` em `events`.
   - Ambas preenchidas na ingestão com a normalização de `internal/listing/people.go`, e por uma migração nos eventos existentes (sem reimportar).
2. **Filtros em SQL.** Os filtros de pessoa e direção retornam **só IDs**. O ranking usa esses IDs com os vetores, e o conteúdo é carregado apenas para o top-k final.
3. **Limite de segurança.** Se o filtro retornar mais de N candidatos (configurável), usar o KNN do vec0 com `k` maior e pós-filtrar pelos IDs.

### Critério de aceite

- Um benchmark com 100 mil eventos e uma pergunta por pessoa sem período mostra que a memória residente não cresce com o tamanho do banco.
- Os resultados são idênticos aos da implementação atual na suíte de recuperação.
- `forget` apaga as linhas de `event_people` (teste de privacidade).

---

## Fase 7 — Evidência não confiável no prompt

### Problema

Mensagens de terceiros (Teams), títulos de páginas e conteúdo de arquivos entram crus no prompt (`internal/rag/prompt.go`, `formatEvidence`). Como o modelo não tem ferramentas, o risco se limita a manipular a resposta, mas uma mensagem com instruções pode distorcer o que o usuário lê.

### Mudanças

1. Delimitar cada evidência, por exemplo `<evento n="3" fonte="teams" autor="Rui Costa">…</evento>`, e neutralizar delimitadores que apareçam dentro do conteúdo.
2. Acrescentar às instruções do sistema que o conteúdo dos eventos é dado, nunca instrução.
3. Usar como teste de regressão os casos de injeção que já estão no corpus da suíte de recuperação.

### Critério de aceite

- Nos casos de injeção, a resposta não segue a instrução embutida e continua citando as fontes corretamente.

---

## Fase 11 — Instalação e configuração

### Problema

- **Muitos pré-requisitos:** o build exige gcc, cmake, ninja, curl e, opcionalmente, o CUDA Toolkit. Depois é preciso baixar os modelos (`make models`) e escrever o `config.json` à mão, com os caminhos de repositórios, históricos e perfis do Teams.
- **Configuração pouco explicada:** o README mostra só um fragmento do `config.json`.
- Para quem não conhece Go nem o llama.cpp, a barreira de entrada é alta.

### Mudanças

1. **Configuração de exemplo.**
   - `config.example.json` completo.
   - Uma tabela no README com cada campo, o padrão e um exemplo. JSON não tem comentários, então a explicação fica na tabela.
2. **`cade init` interativo.**
   - Detecta os históricos do Chrome e do Firefox, os perfis do Teams e os repositórios git sob um diretório informado.
   - Pergunta o que incluir e grava o `config.json` com permissão `600`.
   - Não lê conteúdo, só caminhos.
3. **`cade doctor`.** Verifica os modelos, o FTS5, os caminhos configurados, a versão do esquema e a reindexação pendente, e diz o que corrigir.
4. **Binários prontos** (junto com a Fase 9): builds CPU para Linux anexados à tag, para quem não quer compilar. CUDA continua por build local.

### Critério de aceite

- Documentado e testado: de um clone limpo (ou de um binário baixado) até o primeiro `cade ask`, com a lista de passos no README.
- `cade init` tem testes com um sistema de arquivos falso (históricos e repositórios fictícios).

---

## Fase 12 — Idioma da interface

### Problema

As perguntas funcionam em inglês e português, e `cade help` segue o idioma do sistema. Os demais rótulos da CLI (timeline, tarefas, avisos) são em português, e a documentação técnica é em inglês. Quem não fala português se perde na saída.

### Mudanças

1. Levar os rótulos da saída para o mesmo catálogo de mensagens da ajuda (`internal/cli/usage.go`), seguindo o idioma do sistema.
2. Opção `ui.language` (`auto`, `pt`, `en`) para sobrepor o idioma do sistema.
3. Documentar a regra no README: a interface segue o sistema; a resposta do `ask` segue o idioma da pergunta.

### Critério de aceite

- Com `LANG=en_US.UTF-8`, nenhum rótulo em português na saída de `timeline`, `tasks` e `ask` (teste por comando).
- Os testes de saída atuais continuam passando com `LANG=pt_BR.UTF-8`.

---

## Fase 8 — Documentação e manutenção

1. **README.**
   - Documentar o suporte ao Firefox: `browsersource/flavor.go` já suporta, mas o README só mostra caminhos do Chrome.
   - Adicionar uma tabela de requisitos de hardware com RAM, VRAM e latência em CPU e GPU, tirada de `bench/baseline.txt` e `bench/baseline-cpu.txt` (fase 5).
   - Explicar a deduplicação, os pedaços, a busca híbrida e a autoria no git.
2. **Requisitos referenciados.** O código cita `CA9`, `CA9.1`, `RF4`, `RNF3.1` etc. (34 referências). Conferir que cada uma existe em `docs/USECASES.md`, ou trocá-la por uma explicação no comentário.
3. **Teams.**
   - **Fuzz tests** (`go test -fuzz`) para `internal/leveldbraw`, `internal/v8value` e `internal/indexeddb`. São cerca de 2.200 linhas de parsers de um formato binário não documentado, onde fuzzing encontra problemas com pouco custo.
   - **Aviso de política:** no README e no PRIVACY.md, pedir que o usuário verifique a política de dados da organização antes de ingerir mensagens do Teams, que incluem mensagens de terceiros.
   - **Fragilidade:** a leitura depende do formato interno do IndexedDB do Chrome e do Teams. O `teams-schema` já diagnostica, e a ingestão falha quando não reconhece o formato. Falta guardar amostras anonimizadas de cada formato já visto, como testes de regressão.
4. **Privacidade.** Atualizar PRIVACY.md com a tabela nova da fase 6 (`event_people`) e como ela é apagada. O estado do prompt salvo da fase 5 já está lá.
5. **CHANGELOG.** Mantê-lo em dia a cada entrega, com as migrações novas e o que cada uma reescreve.

---

## Fase 9 — Empacotamento da versão

1. **Número de versão.**
   - `cade version` (e `--version`) mostra a versão, o commit, a data e o tipo de build (CPU/CUDA), injetados com `-ldflags -X` pelo Makefile a partir de `git describe`.
   - Tag git `vX.Y.Z` no commit do lançamento.
2. **Avisos de terceiros.**
   - O projeto é GPLv2, e as dependências embutidas no binário são compatíveis:
     - llama.cpp (MIT);
     - mattn/go-sqlite3 (MIT, com o SQLite em domínio público);
     - sqlite-vec (MIT/Apache-2.0; o módulo Go não traz o arquivo de licença);
     - klauspost/compress (BSD-3).
   - Os avisos dessas licenças precisam ir junto, em `THIRD_PARTY_NOTICES.md`.
3. **Modelos: licença e alternativas.**
   - **Embedding:** o nomic-embed-text-v2-moe é Apache-2.0 (está no GGUF).
   - **Geração:** o GGUF do Qwen2.5-3B-Instruct não traz licença. A conferir no model card: o 3B parece estar sob a Qwen Research License, de uso não comercial, diferente dos outros tamanhos. O resultado vai para o README.
   - **Alternativa:** se a licença restringir o uso, avaliar um modelo de geração com licença aberta na suíte de plano (por exemplo, o Qwen2.5-1.5B, se for Apache-2.0) e documentar as opções menores para máquinas com pouca memória ou disco. O binário não inclui os modelos: `make models` baixa.
4. **Notas de versão.** Avisar de dois pontos:
   - a migração com cópia leva ~1 minuto e grava uma cópia do tamanho do banco;
   - o `cade reindex` é obrigatório depois da migração 5, porque os textos longos ficam sem vetor até ele rodar.

---

## A definir

Outras ideias entram aqui antes de virar fase: problema, mudança proposta e critério de aceite, como nas fases acima.

### Modo daemon para o `ask`

Ficou fora da fase 5 para ser reavaliado depois das medições.

- **Problema:** mesmo com o estado do prompt salvo e as regras, cada `cade ask` carrega os dois modelos. Com o cache de página frio, isso domina na GPU (7,7 s de 8,5 s). Em CPU, o que domina é ler as evidências da resposta (~20 s), que um daemon não evita.
- **Mudança possível:** um processo que mantém os modelos carregados, atendendo o `cade ask` por um socket Unix com permissão só do dono, e que encerra depois de um tempo parado.
- **A verificar:** se vale a memória ocupada o tempo todo (~2,5 GB de GPU ou ~3,9 GB de RAM), e se o ganho em CPU justifica, já que ali o custo é gerar, não carregar.

### Busca em PDFs e imagens (avaliar depois das fases)

Verificar se faz sentido depois de terminar as fases acima. Não entra na ordem da release.

#### PDFs

- **Encaixe:** é uma extensão natural do pipeline atual, na sequência extração de texto → pedaços (`internal/chunking`) → `TextEmbedder` → `sqlite-vec` e `chunks_fts`.
- **Metadados:** arquivo, página, autor e data. A página permite citar na resposta ("relatorio.pdf, p. 12").
- **OCR:** PDFs escaneados exigem OCR (Tesseract) antes dos pedaços.

#### Imagens: detector + descrição

Pipeline: imagem → detector (YOLO/DETR) → lista de objetos → descrição por template → `TextEmbedder` → `sqlite-vec`.

**Por quê:**
- Reaproveita o pipeline de texto e o `sqlite-vec` por inteiro.
- Custo baixo: detectores pequenos rodam em CPU, sem GPU nem modelo de visão e linguagem.
- É determinístico e fácil de depurar, porque a lista crua fica no banco.
- Combina com OCR: a descrição e o texto do OCR viram pedaços de texto.

**Descrição, em passos:**
1. Template fixo: "Imagem com 3 pessoas, 1 carro, 2 árvores."
2. Template com posição e confiança: a caixa de cada objeto vira uma posição aproximada.
3. Opcional: pós-processamento em lote com o LLM local (Qwen2.5-3B).

**Limitações:**
- O detector só vê as classes em que foi treinado (80 no COCO). Objetos abstratos exigem OCR ou legenda.
- Poucos atributos: cor, ação e expressão ficam de fora.
- Relações espaciais simplificadas.
- O limiar de confiança precisa de calibração.

**Esquema sugerido:**
- `media` (path, type, hash, created_at);
- vetores com `modality` (`text`, `pdf` ou `image`);
- campos por mídia: `detections_json`, `caption` e `ocr_text`.

**A verificar antes de virar fase:**
- **Encaixe no modelo atual:** PDFs e imagens poderiam ser eventos da fonte de arquivos, com pedaços e o `content_hash` que já existem. Nesse caso bastaria guardar a página e as detecções no metadado, sem uma tabela `media` nem vetores separados por modalidade. Toda mudança de esquema continua sendo uma migração.
- **Runtime:** o llama.cpp não roda YOLO nem DETR. Seria preciso outra biblioteca de inferência (por exemplo, o ONNX Runtime) via cgo, embutida no binário como o llama.cpp, sem rede.
- **Licenças:**
  - os pesos e o código do YOLO da Ultralytics são AGPL-3.0, incompatível com a GPLv2 do cade;
  - o DETR é Apache-2.0, e o Tesseract também.
- **Avaliação:** casos novos na suíte de recuperação (perguntas sobre o conteúdo de PDFs e imagens) e custo de ingestão por página e por imagem em CPU.
- **Privacidade:** o texto extraído e as detecções passam a ficar no banco; PRIVACY e `forget` precisam cobri-los.

---

## Critérios de release

A versão sai quando:

- [ ] a CI está verde no commit da tag;
- [ ] `make test` e `make eval` passam, com as métricas reportadas no conjunto de **teste**;
- [ ] recall, MRR e rejeição no teste ficam iguais ou melhores que o baseline da Fase 0, em todos os tamanhos da curva de escala;
- [ ] todas as migrações que reescrevem dados fazem backup antes e têm teste em `migrations_test.go`;
- [ ] `forget` apaga os dados de todas as tabelas novas (teste de privacidade);
- [ ] o baseline de benchmark é atualizado com GPU, CPU e cold start;
- [ ] README, PRIVACY e CHANGELOG estão atualizados, em inglês e português;
- [ ] um usuário novo chega ao primeiro `cade ask` seguindo só o README;
- [ ] `cade version` mostra a versão da tag, e `THIRD_PARTY_NOTICES.md` e a licença dos modelos estão no repositório;
- [ ] as notas de versão avisam da migração com cópia e do `cade reindex` obrigatório;
- [ ] nenhum nome real de pessoa, cliente ou empresa em código, testes, corpus ou documentação.
