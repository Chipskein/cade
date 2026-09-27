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
| 10 | [Integração contínua e qualidade](#fase-10--integração-contínua-e-qualidade) | concluída | alto | baixo |
| 6 | Filtros de pessoa no SQL | concluída | médio | baixo |
| 7 | [Evidência não confiável no prompt](#fase-7--evidência-não-confiável-no-prompt) | concluída; 1 caso de injeção ainda falha | baixo | baixo |
| 11 | [Instalação e configuração](#fase-11--instalação-e-configuração) | concluída; binários prontos passam para a fase 9 | alto | médio |
| 12 | [Idioma da interface](#fase-12--idioma-da-interface) | concluída | médio | médio |
| 8 | [Documentação e manutenção](#fase-8--documentação-e-manutenção) | concluída | baixo | baixo |
| 9 | [Empacotamento da versão](#fase-9--empacotamento-da-versão) | concluída; v0.0.0 publicada em 2026-09-27 | pré-requisito do lançamento | baixo |
| — | [A definir](#a-definir) | em aberto | — | — |

A tabela está na ordem sugerida:
- **CI (10) primeiro:** é barata e protege todas as fases seguintes. Entregue.
- **Depois as mudanças de código:** 6 e 7 (entregues). A 5 foi feita antes da 10, a pedido.
- **Em seguida, a experiência de quem instala:** 11 e 12 (entregues).
- **Docs (8, entregue) e empacotamento (9) por último:** descrevem o estado final.

---

## Fase 10 — Integração contínua e qualidade

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)). A primeira execução no GitHub, com o cache frio, passou em 4 min 04 s (o job de testes levou 3 min 41 s). Registrar quando houver:

- o tempo com o cache quente (a partir do push seguinte);
- um PR que quebra um teste ou o `gofmt` ficando vermelho;
- quanto o `eval.yml` leva em CPU, rodado uma vez à mão.

---

## Fase 7 — Evidência não confiável no prompt

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)), com uma diferença do plano: as tags `<evento>` foram medidas e pioraram, então cada evento que dá ordens ao assistente é marcado no cabeçalho, e a regra do prompt se refere à marca. Falta:

- o caso da nota que finge ser "nova instrução do sistema" ainda é seguido pelo modelo de 3B (`make eval-injection` fica vermelho nesse caso). Tirar o evento do prompt zera as injeções seguidas; a outra saída é um modelo de geração que siga a regra (ver [Migrar a geração para o Qwen3.5](#migrar-a-geração-para-o-qwen35)).

---

## Fase 11 — Instalação e configuração

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)): `config.example.json` e a tabela completa no README, `cade init` interativo, `cade doctor` e o passo a passo do clone ao primeiro `cade ask`. Os binários prontos foram para a [fase 9](#fase-9--empacotamento-da-versão), porque dependem do número de versão e da tag.

---

## Fase 12 — Idioma da interface

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)): os rótulos seguem o idioma do sistema ou `ui.language`, com testes por comando em inglês. O prompt do modelo e os códigos do `ask --json` ficaram em português de propósito.

---

## Fase 8 — Documentação e manutenção

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)): README com hardware, navegadores e como a busca trata o histórico; referências a requisitos conferidas; fuzz tests e amostra do formato do Teams (que acharam um pânico no coletor); aviso de política de dados; CA10 verificado sem rede. O CHANGELOG segue sendo atualizado a cada entrega.

---

## Fase 9 — Empacotamento da versão

Entregue (ver o [CHANGELOG](../CHANGELOG.pt-BR.md)): `cade version`, `THIRD_PARTY_NOTICES.md`, licenças dos modelos (o Qwen2.5-3B é só para uso não comercial; o 1.5B, Apache-2.0, ficou abaixo do piso da suíte de plano), `make dist` e workflow de release. A [v0.0.0](https://github.com/Chipskein/cade/releases/tag/v0.0.0) saiu em 2026-09-27, do `master`. Para as próximas versões:

1. Trocar o cabeçalho do topo do CHANGELOG (EN/PT) pela versão e data, e escrever `docs/release-notes/vX.Y.Z.md`.
2. Conferir os critérios de release abaixo.
3. Levar o `dev` para o `master` e criar a tag lá: `git tag vX.Y.Z origin/master && git push origin vX.Y.Z`. O workflow testa, gera o binário e publica o release; uma tag fora do `master` falha sem publicar. O `dev` recebe o trabalho do dia a dia e não gera releases.

---

## A definir

Outras ideias entram aqui antes de virar fase: problema, mudança proposta e critério de aceite, como nas fases acima.

### Migrar a geração para o Qwen3.5

Hoje o modelo de geração é o Qwen2.5-3B-Instruct (Q4_K_M). A proposta é trocá-lo por um Qwen3.5 de tamanho parecido.

- **Por quê:**
  - o 2.5-3B não segue de forma confiável a regra contra injeção (fase 7: um caso ainda falha, e a redação da regra muda o resultado);
  - às vezes não cita a evidência;
  - a licença do 3B está em dúvida (fase 9).
- **A verificar antes de virar fase:**
  - **Suporte:** o llama.cpp fixado (`LLAMA_TAG` b11195) carrega a arquitetura e o template de chat do Qwen3.5? Se não, subir a tag, rodando `make test-models` e `make bench`.
  - **Tamanho e memória:** qual variante cabe no mesmo orçamento (~2,5 GB de GPU, ~3,9 GB de RAM em CPU) e qual a latência do `ask` em CPU e GPU (`BenchmarkColdAsk`).
  - **Raciocínio:** se o modelo tem modo de raciocínio, ele precisa ficar desligado ou fora da resposta, para não gastar tokens nem vazar texto no `ask`.
  - **Licença:** conferir no model card e registrar no README (fase 9).
  - **Qualidade:** `make eval` completo, comparado com o 2.5-3B:
    - a suíte de plano (a saída restrita por gramática GBNF precisa continuar funcionando);
    - a de injeção;
    - as citações.
  - **Prompt salvo:** o estado salvo do planejador é invalidado sozinho, porque a chave inclui o arquivo do modelo.
- **Critério de aceite:** plano igual ou melhor por campo, nenhuma injeção seguida em `make eval-injection`, latência e memória dentro do orçamento, e licença que permita o uso.

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
