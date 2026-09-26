# Privacidade

[English](PRIVACY.md) · **Português**

O cade lê seus commits, histórico do navegador, arquivos e mensagens do Teams, então guarda alguns dos dados mais pessoais da máquina. Esta página lista o que ele guarda, onde, o que o modelo local vê e como apagar.

## Resumo

- **Nada sai da máquina durante o uso.** O binário não tem código de rede: `net` e `net/http` do Go não entram no build, e o llama.cpp é compilado sem o downloader. Não há telemetria.
- **A rede só é usada no build:** `make llama` clona o llama.cpp do GitHub e `make models` baixa os dois modelos do Hugging Face.
- **Tudo fica num arquivo SQLite**, legível só pelo seu usuário (permissão `600`, pasta `700`).
- **O banco não é criptografado.** Quem estiver logado como você, ou tiver o seu disco, consegue lê-lo. Use criptografia de disco.

## Onde os dados ficam

| Caminho | Conteúdo | Permissão |
|---|---|---|
| `~/.local/share/cade/cade.db` (+ `-wal`, `-shm`) | eventos, texto, pedaços (posições no texto), embeddings, índice das palavras de cada pedaço (mais hashes de commit e caminhos de arquivo), histórico de edição de arquivos | `600` |
| `~/.local/share/cade/cade.db.before-v*` | cópia salva antes de uma migração de esquema que reescreve dados; mesmo conteúdo do banco | `600` |
| `~/.config/cade/config.json` | quais repositórios, históricos, pastas e perfis do Teams ler | `600` |
| `~/.local/share/cade/models/` | os dois modelos (arquivos públicos) | — |
| `/tmp/cade-browser-*`, `/tmp/cade-indexeddb-*` | cópias do histórico do navegador e do IndexedDB do Teams durante uma ingestão; apagadas ao final | `700` |

Backups da sua pasta pessoal incluem o `cade.db`.

## O que cada fonte guarda

| Fonte | Guarda | Não guarda |
|---|---|---|
| **git** | mensagem do commit, nomes dos arquivos alterados, caminho do repositório, hash, nome e e-mail do autor, data | diffs, conteúdo dos arquivos |
| **navegador** | toda visita do arquivo de histórico: URL (com a query string, que às vezes carrega tokens), título da página, data | conteúdo das páginas, cookies, senhas, formulários; janelas anônimas não entram no histórico |
| **arquivos** | para cada arquivo nas pastas configuradas: caminho, tamanho, data de modificação e, para arquivos de texto UTF-8 de até `max_file_bytes` (256 KB), o **texto inteiro** | conteúdo de binários; pastas em `ignored_dir_names` (`.git`, `node_modules`…) |
| **teams** | mensagens de chat, canal e chat de reunião: texto (também guardado sozinho, para remontar o conteúdo), nome e id do remetente, id e título da conversa, se foi você que enviou, versão de edição | agenda, histórico de chamadas, notificações, arquivos; mensagens apagadas antes da ingestão |

Escolha as `directories` com cuidado: um `.env` ou uma nota com senhas dentro delas fica guardado como texto.

## Teams

- As mensagens vêm do IndexedDB que o Teams na web mantém no Chrome. O cade copia a pasta inteira do IndexedDB para uma pasta temporária (os arquivos em uso ficam travados), decodifica só os stores de mensagens, conversas e perfis, e apaga a cópia.
- Só existe o que o cliente do Teams tem em cache. Mensagens antigas não são buscadas, porque o cade nunca fala com a Microsoft.
- Uma mensagem editada substitui o texto guardado na próxima ingestão. Uma mensagem apagada no Teams depois de ingerida **continua** no cade até você rodar `cade forget teams`.
- `cade teams-schema` mostra a estrutura de um IndexedDB (stores, campos, tipos, contagens) sem nenhum valor, então a saída pode ser compartilhada para diagnóstico.

## O que o modelo local vê

Os dois modelos rodam dentro do processo, pelo llama.cpp.

- **Modelo de embedding:** o texto inteiro de cada evento, em pedaços de até 1.200 caracteres, na ingestão (texto idêntico é embutido uma vez), e o texto de busca de cada pergunta.
- **Interpretação da pergunta:** só a sua pergunta, com instruções e exemplos fixos.
- **Respostas:** a sua pergunta, a data de hoje e até `top_k` (8) eventos; de um evento longo, só o pedaço que casou (até 1.200 caracteres), senão o texto até esse tamanho.
- Nada mais do banco é passado ao modelo. Listagens e relatórios de tarefas não usam o modelo de geração; uma listagem com assunto ("páginas sobre redis") só embute o assunto.

## Logs

- Os logs vão só para o stderr; nada é gravado em arquivo de log.
- `--verbose` acrescenta linhas de depuração com a resposta do modelo, os títulos dos eventos recuperados e a pergunta interpretada. Não cole isso em lugar público.

## Apagar dados

| Objetivo | Comando |
|---|---|
| Apagar uma fonte | `cade forget teams` (ou `git`, `browser`, `file`) |
| Apagar tudo | `rm ~/.local/share/cade/cade.db*` |
| Remover a configuração | `rm -r ~/.config/cade` |

- O `forget` apaga os eventos e seus embeddings, depois compacta o arquivo e esvazia o log de escrita (WAL). Assim o texto apagado sai do disco, em vez de ficar em páginas livres.
- O índice de palavras acompanha o texto: uma edição, o `reindex` e o `forget` também tiram dele as palavras antigas.
- Texto substituído, como o de uma mensagem editada ou de uma versão antiga de um arquivo, também é zerado (`secure_delete` do SQLite). Só a versão atual de cada arquivo fica guardada; `file_modifications` guarda a data e o tamanho de cada versão anterior, sem o texto, e o `forget file` a apaga.
- Um arquivo apagado da pasta sai das respostas, mas continua no banco (e na timeline) até `cade forget file`.
- O `forget` não mexe nas cópias de migração (`cade.db.before-v*`); apague-as você mesmo.
- Cópias feitas fora do cade não são afetadas: backups, snapshots, ou saídas de `cade ask --json` que você salvou.
- Não há comando para apagar um evento isolado.
