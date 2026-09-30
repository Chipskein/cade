---
name: Tarefa
about: Uma mudança planejada, dividida em tasks de até 10 arquivos (AGENTS.md)
title: ""
labels: enhancement
---

<!-- Apague os comentários e as seções que não se aplicam. -->

## Problema

<!-- O que está errado ou falta hoje, e para quem. Cite o código (`pacote/arquivo.go`) quando ajudar. -->

## Medição

<!-- Opcional. Números de hoje e como foram obtidos (comando, máquina, data), para comparar depois.

| | Hoje |
|---|---|
| | |
-->

## Proposta

<!-- O que muda e por quê. Riscos e o que fica igual (dados gravados, privacidade, `ask --json`). -->

## Tasks

<!-- Uma task = um commit que passa no `go tool mage check` sozinho, com no máximo 10 arquivos contando testes e docs (AGENTS.md). Liste os arquivos de cada uma. -->

1. <!-- núcleo + testes: `...` -->
2. <!-- CLI + testes: `...` -->
3. <!-- docs (README, docs/, CHANGELOG, PRIVACY em EN e PT): `...` -->

## Critério de aceite

- [ ] <!-- resultado verificável, medido do mesmo jeito que a medição acima -->
- [ ] `go tool mage check` verde em cada commit
- [ ] `go tool mage eval` quando afetar busca ou plano; `go tool mage bench` quando afetar desempenho
- [ ] docs em EN e PT; PRIVACY quando tocar dados; migração (nunca `forget` + `ingest`) quando mudar o esquema
