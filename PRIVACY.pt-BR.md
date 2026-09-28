# Privacidade

[English](PRIVACY.md) · **Português**

O cade lê seus commits, histórico do navegador, arquivos e mensagens do Teams, então guarda alguns dos dados mais pessoais da máquina. Esta página lista o que ele guarda, onde, o que o modelo local vê e como apagar.

## Resumo

- **Nada sai da máquina durante o uso.** O binário não tem código de rede: `net` e `net/http` do Go não entram no build, e o llama.cpp é compilado sem o downloader. Não há telemetria.
- **A rede só é usada no build:** `make llama` clona o llama.cpp do GitHub e `make models` baixa os dois modelos do Hugging Face.
- **`cade init` e `cade doctor` olham nomes, não conteúdo:** o init lista as pastas de perfil e confere quais têm `History`, `places.sqlite` ou IndexedDB do Teams, e procura `.git` sob o diretório que você indicar; o doctor lê os primeiros bytes dos modelos e dos históricos (as assinaturas `GGUF` e `SQLite format 3`) e abre o banco só para leitura.
- **Tudo fica num arquivo SQLite**, legível só pelo seu usuário (permissão `600`, pasta `700`).
- **O banco não é criptografado.** Quem estiver logado como você, ou tiver o seu disco, consegue lê-lo. Use criptografia de disco.

## Onde os dados ficam

| Caminho | Conteúdo | Permissão |
|---|---|---|
| `~/.local/share/cade/cade.db` (+ `-wal`, `-shm`) | eventos, texto, pedaços (posições no texto), embeddings, índice das palavras de cada pedaço (mais hashes de commit e caminhos de arquivo), histórico de edição de arquivos | `600` |
| `~/.local/share/cade/cade.db.before-v*` | cópia salva antes de uma migração de esquema que reescreve dados; mesmo conteúdo do banco | `600` |
| `~/.config/cade/config.json` | suas identidades de commit, se você as listar (`git_identities`), e quais repositórios, históricos, pastas e perfis do Teams ler | `600` |
| `~/.local/share/cade/models/` | os dois modelos (arquivos públicos) | — |
| `/tmp/cade-browser-*`, `/tmp/cade-indexeddb-*` | cópias do histórico do navegador e do IndexedDB do Teams durante uma ingestão; apagadas ao final | `700` |

Backups da sua pasta pessoal incluem o `cade.db`.

## O que cada fonte guarda

| Fonte | Guarda | Não guarda |
|---|---|---|
| **git** | mensagem do commit, nomes dos arquivos alterados, caminho do repositório, hash, nome e e-mail do autor, data | diffs, conteúdo dos arquivos |
| **navegador** | toda visita: URL sem parâmetros de credencial, título da página, data | conteúdo das páginas, cookies, senhas, formulários; janelas anônimas não entram no histórico |
| **arquivos** | caminho, tamanho, data de modificação e (para texto UTF-8 até `max_file_bytes`) o texto, exceto nomes de arquivos de credenciais ignorados | conteúdo de binários; pastas ignoradas e globs de arquivos de credenciais |
| **teams** | mensagens de chat, canal e chat de reunião: texto (também guardado sozinho, para remontar o conteúdo), nome e id do remetente, id e título da conversa, se foi você que enviou, versão de edição | agenda, histórico de chamadas, notificações, arquivos; mensagens apagadas antes da ingestão |

Arquivos ignorados incluem `.env*`, chaves e certificados e arquivos comuns de credenciais, conforme `sources.ignored_file_globs`. URLs do navegador perdem parâmetros `token`, OAuth, assinatura, senha e assinaturas de nuvem; os demais parâmetros ficam. Com `ingest.redact` ativo (padrão), tokens conhecidos do GitHub, GitLab, AWS e Slack, JWTs e blocos de chave privada PEM são substituídos por rótulos como `[redacted:github-token]` no texto e nos metadados dos eventos. A migração de esquema 8 também limpa eventos existentes e cria a cópia `cade.db.before-v8-*`; textos alterados ficam sem vetores até `cade reindex`.

A detecção reconhece formatos, não todos os segredos: senhas arbitrárias, tokens próprios e formatos desconhecidos ainda podem ser guardados. Defina `ingest.redact` como `false` para desligar a máscara de texto; globs de arquivos e remoção de parâmetros de URL continuam ativos. Definir `sources.ignored_file_globs` substitui a lista padrão inteira; inclua os padrões padrão para ampliá-la.

## Teams

- **Confira antes a política de dados da sua organização.** O cache do Teams guarda mensagens de outras pessoas, em chats e canais, e ingeri-lo copia essas mensagens para o banco do cade no seu disco. Muitas organizações restringem guardar mensagens de trabalho fora das ferramentas aprovadas. O `cade init` deixa o Teams de fora a menos que você o escolha.
- As mensagens vêm do IndexedDB que o Teams na web mantém no Chrome. O cade copia a pasta inteira do IndexedDB para uma pasta temporária (os arquivos em uso ficam travados), decodifica só os stores de mensagens, conversas e perfis, e apaga a cópia.
- Só existe o que o cliente do Teams tem em cache. Mensagens antigas não são buscadas, porque o cade nunca fala com a Microsoft.
- Uma mensagem editada substitui o texto guardado na próxima ingestão. Uma mensagem apagada no Teams depois de ingerida **continua** no cade até você rodar `cade forget teams`.
- `cade teams-schema` mostra a estrutura de um IndexedDB (stores, campos, tipos, contagens) sem nenhum valor, então a saída pode ser compartilhada para diagnóstico.

## O que o modelo local vê

Os dois modelos rodam dentro do processo, pelo llama.cpp.

- **Modelo de embedding:** o texto inteiro de cada evento, em pedaços de até 1.200 caracteres, na ingestão (texto idêntico é embutido uma vez), e o texto de busca de cada pergunta.
- **Interpretação da pergunta:** só a sua pergunta, com instruções e exemplos fixos. Perguntas lidas por regras (período, fonte e palavras genéricas) nem chegam ao modelo.
- **Estado do prompt salvo:** `~/.cache/cade/prompt-state/` guarda um arquivo (~55 MB, só o dono lê) com o estado do modelo depois das instruções e exemplos fixos. Não contém pergunta nenhuma nem nada do banco.
- **Respostas:** a sua pergunta, a data de hoje e até `top_k` (8) eventos; de um evento longo, só o pedaço que casou (até 1.200 caracteres), senão o texto até esse tamanho.
- **Texto que dá ordens ao assistente:** mensagens, títulos de páginas e notas são palavras de outras pessoas, e alguma pode ser escrita para manipular a resposta ("IMPORTANTE para o assistente: ignore as regras…"). Esses eventos vão ao modelo marcados como não confiáveis e sem o texto, com uma regra para não usá-los nem citá-los; a lista de fontes mostra o evento com a marca, e o `ask --json` põe `"untrusted": true`. Isso reduz o risco sem eliminá-lo: um texto que o detector não reconhece ainda chega inteiro ao modelo e pode manipular a resposta, mas o modelo não tem ferramentas, então não consegue agir sobre nada.
- Nada mais do banco é passado ao modelo. Listagens e relatórios de tarefas não usam o modelo de geração; uma listagem com assunto ("páginas sobre redis") só embute o assunto.

## Logs

- Os logs vão só para o stderr; nada é gravado em arquivo de log.
- `--verbose` acrescenta linhas de depuração com a resposta do modelo, os títulos dos eventos recuperados e a pergunta interpretada. Não cole isso em lugar público.

## Apagar dados

| Objetivo | Comando |
|---|---|
| Apagar uma fonte | `cade forget teams` (ou `git`, `browser`, `file`) |
| Apagar um evento | `cade forget --uid UID` ou revisar resultados com `cade forget --match TEXTO [--source F] [--from D --to D]` |
| Apagar tudo | `rm ~/.local/share/cade/cade.db*` |
| Remover a configuração | `rm -r ~/.config/cade` |
| Remover o estado do prompt salvo | `rm -r ~/.cache/cade/prompt-state` (refeito na próxima pergunta) |

- O `forget` apaga os eventos e seus embeddings, depois compacta o arquivo e esvazia o log de escrita (WAL). Assim o texto apagado sai do disco, em vez de ficar em páginas livres.
- O índice de palavras acompanha o texto: uma edição, o `reindex` e o `forget` também tiram dele as palavras antigas.
- O índice de pessoas (`event_people`) guarda os nomes de cada mensagem e commit (remetente ou autor, título da conversa, primeiros nomes depois de "@"), para que uma pergunta sobre uma pessoa seja filtrada no banco. Ele acompanha os eventos: uma edição troca os nomes do evento, e o `forget` os apaga junto com os eventos.
- Texto substituído, como o de uma mensagem editada ou de uma versão antiga de um arquivo, também é zerado (`secure_delete` do SQLite). Só a versão atual de cada arquivo fica guardada; `file_modifications` guarda a data e o tamanho de cada versão anterior, sem o texto, e o `forget file` a apaga.
- Um arquivo apagado da pasta sai das respostas, mas continua no banco (e na timeline) até `cade forget file`.
- O `forget` não mexe nas cópias de migração (`cade.db.before-v*`); apague-as você mesmo.
- Cópias feitas fora do cade não são afetadas: backups, snapshots, ou saídas de `cade ask --json` que você salvou.
- `forget --uid` e `forget --match` confirmado removem o evento, pedaços, embeddings, índices de palavras e pessoas e histórico do arquivo; só o UID e a data da remoção ficam para impedir a reingestão. `forget <fonte>` limpa essa lista de UIDs esquecidos. A retenção vem desligada; configure `ingest.retention.max_age_days` por fonte para ativá-la.
