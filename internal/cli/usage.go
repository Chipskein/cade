package cli

const usageText = `cade — histórico pessoal local (git, browser, arquivos, teams)

Uso:
  cade [--config ARQUIVO] [--verbose] <comando> [argumentos]

Comandos:
  init                                  cria o arquivo de configuração padrão
  ingest <fonte|all> [ALVO...]          ingere uma fonte (git, browser, file, teams);
                                        sem ALVO usa os alvos configurados
  timeline [--source F] DATA [DATA_FIM] eventos de um dia ou intervalo
                                        (DATA: AAAA-MM-DD, hoje, ontem)
  ask [--source F] [--from D] [--to D] PERGUNTA
                                        pergunta em linguagem natural
  forget <fonte>                        apaga os eventos de uma fonte, para reingerir;
                                        o que já saiu da fonte (ex.: cache do Teams
                                        expirado) não volta
  teams-schema DIR...                   estrutura (sem valores) de um IndexedDB
                                        do Chrome, p/ desenhar o ingestor do Teams

Exemplos:
  cade ingest git ~/src/meu-projeto
  cade ingest browser ~/.mozilla/firefox/xyz.default/places.sqlite
  cade ingest all
  cade timeline ontem
  cade timeline 2026-09-01 2026-09-07
  cade ask --source browser --from 2026-09-19 "o que pesquisei sobre sqlite?"
`
