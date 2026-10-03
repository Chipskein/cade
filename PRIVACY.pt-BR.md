<p align="center">
  <img src="https://raw.githubusercontent.com/Chipskein/cade/dev/assets/cade2.png" alt="cade mascot: a Go gopher filing folders" width="200">
</p>

# Privacidade

[English](PRIVACY.md) · **Português**

O cade lê seus commits, histórico do navegador, arquivos e mensagens do Teams, então guarda alguns dos dados mais pessoais da máquina. Esta página lista o que ele guarda, onde, o que o modelo local vê e como apagar.

## Resumo

- **Nada sai da máquina durante o uso.** O binário não tem código de rede: `net` e `net/http` do Go não entram no build, e o llama.cpp é compilado sem o downloader. Não há telemetria.
- **A rede só é usada no build:** `go tool mage llama` clona o llama.cpp do GitHub e `go tool mage models` baixa os dois modelos e o projetor de visão do modelo de geração do Hugging Face.
- **`cade init` e `cade doctor` olham nomes, não conteúdo:** o init lista as pastas de perfil e confere quais têm `History`, `places.sqlite` ou IndexedDB do Teams, e procura `.git` sob o diretório que você indicar; o doctor lê os primeiros bytes dos modelos e dos históricos (as assinaturas `GGUF` e `SQLite format 3`) e abre o banco só para leitura.
- **Tudo fica num arquivo SQLite**, legível só pelo seu usuário (permissão `600`, pasta `700`).
- **O banco não é criptografado.** Quem estiver logado como você, ou tiver o seu disco, consegue lê-lo. Use criptografia de disco.

## Onde os dados ficam

| Caminho | Conteúdo | Permissão |
|---|---|---|
| `~/.local/share/cade/cade.db` (+ `-wal`, `-shm`) | eventos, texto, pedaços (posições no texto, um conjunto por texto distinto), embeddings, índice das palavras de cada pedaço, índice de hashes de commit e caminhos de arquivo, histórico de edição de arquivos | `600` |
| `~/.local/share/cade/cade.db.before-v*` | cópia salva antes de uma migração de esquema que reescreve dados; mesmo conteúdo do banco | `600` |
| `~/.config/cade/config.json` | suas identidades de commit, se você as listar (`git_identities`), e quais repositórios, históricos, pastas, perfis do Teams e diretórios de IndexedDB ler | `600` |
| `~/.config/cade/idb-schemas/` | um schema por aplicativo: nomes de bancos e stores, caminhos dos campos e seus tipos, as condições que você ou o modelo escreveram (como valores de tipo de mensagem); nenhuma mensagem, nome ou id. `history/` guarda as revisões substituídas, `*.candidate.json` as recusadas | `600` |
| `~/.local/share/cade/cade.db.before-rekey-*` | cópia salva antes de o `schema-check --rekey` renomear UIDs; mesmo conteúdo do banco | `600` |
| `~/.local/share/cade/models/` | os dois modelos e o projetor de visão (arquivos públicos) | — |
| `~/.local/state/cade/ingest-state.json` | o `ingest` em andamento ou o último: argumentos (fonte e alvos), etapa, última linha de progresso e erro | `600` |
| `~/.local/state/cade/ingest.log` | saída do último `ingest start`: linhas de progresso com os alvos, o relatório, erros; substituído a cada execução em segundo plano | `600` |
| `/tmp/cade-browser-*`, `/tmp/cade-indexeddb-*`, `/tmp/cade-firefoxidb-*`, `/tmp/cade-firefoxcache-*`, `/tmp/cade-firefoxstorage-*` | cópias do histórico do navegador, de um IndexedDB (Chromium ou Firefox), do banco da Cache API do Firefox e do banco do localStorage ou do OPFS do Firefox enquanto um comando o lê; apagadas ao final | `700` |

Backups da sua pasta pessoal incluem o `cade.db`.

## O que cada fonte guarda

| Fonte | Guarda | Não guarda |
|---|---|---|
| **git** | mensagem do commit, nomes dos arquivos alterados, caminho do repositório, hash, nome e e-mail do autor, data | diffs, conteúdo dos arquivos |
| **navegador** | toda visita: URL sem parâmetros de credencial, título da página, data | conteúdo das páginas, cookies, senhas, formulários; janelas anônimas não entram no histórico |
| **arquivos** | caminho, tamanho, data de modificação e (para texto UTF-8 até `max_file_bytes`) o texto, exceto nomes de arquivos de credenciais ignorados; com `sources.images` ligado, uma descrição de cada imagem png, jpeg e webp escrita pelo modelo local (o que ela mostra e o texto visível nela), com o SHA-256 da imagem, o tamanho em pixels e o modelo e a versão do prompt | conteúdo de binários, pixels das imagens; pastas ignoradas e globs de arquivos de credenciais |
| **teams** | mensagens de chat, canal e chat de reunião: texto (também guardado sozinho, para remontar o conteúdo), nome e id do remetente, id e título da conversa, se foi você que enviou, versão de edição | agenda, histórico de chamadas, notificações, arquivos; mensagens apagadas antes da ingestão |
| **aplicativos por schema** (ex.: `whatsapp`) | o que o schema mapeia: ids da mensagem e da conversa, nome da conversa, id e nome do remetente, hora, se foi você que enviou, o texto quando o aplicativo o guarda legível, o nome e a revisão do schema | qualquer outra coisa do IndexedDB do aplicativo; no WhatsApp Web, o texto (cifrado pelo aplicativo, nunca decifrado) e as mídias |

Arquivos ignorados incluem `.env*`, chaves e certificados e arquivos comuns de credenciais, conforme `sources.ignored_file_globs`. URLs do navegador perdem parâmetros `token`, OAuth, assinatura, senha e assinaturas de nuvem; os demais parâmetros ficam. Com `ingest.redact` ativo (padrão), tokens conhecidos do GitHub, GitLab, AWS e Slack, JWTs e blocos de chave privada PEM são substituídos por rótulos como `[redacted:github-token]` no texto e nos metadados dos eventos. A migração de esquema 8 também limpa eventos existentes e cria a cópia `cade.db.before-v8-*`; textos alterados ficam sem vetores até `cade reindex`.

A detecção reconhece formatos, não todos os segredos: senhas arbitrárias, tokens próprios e formatos desconhecidos ainda podem ser guardados. Defina `ingest.redact` como `false` para desligar a máscara de texto; globs de arquivos e remoção de parâmetros de URL continuam ativos. Definir `sources.ignored_file_globs` substitui a lista padrão inteira; inclua os padrões padrão para ampliá-la.

## Imagens

Desligado por padrão; o `cade init` pergunta, e `sources.images` liga.

- **A descrição toma o lugar da imagem.** Ela passa pela mesma máscara de segredos que qualquer texto: um print de terminal com um token guarda `[redacted:github-token]`. As pastas e os globs ignorados valem para imagens também.
- **O SHA-256 faz uma imagem movida ou renomeada reaproveitar a descrição**, sem ser descrita de novo. Ele identifica os bytes do arquivo, não o que a imagem mostra.
- **Texto dentro de uma imagem é de outra pessoa,** como uma mensagem: o modelo é instruído a copiá-lo, não a obedecê-lo, e uma descrição cujo texto dá ordens ao assistente é marcada como não confiável no `ask`, como abaixo.

## Teams

- **Confira antes a política de dados da sua organização.** O cache do Teams guarda mensagens de outras pessoas, em chats e canais, e ingeri-lo copia essas mensagens para o banco do cade no seu disco. Muitas organizações restringem guardar mensagens de trabalho fora das ferramentas aprovadas. O `cade init` deixa o Teams de fora a menos que você o escolha.
- As mensagens vêm do IndexedDB que o Teams na web mantém no Chrome, no Firefox ou no Floorp. O cade copia a pasta inteira do IndexedDB para uma pasta temporária (os arquivos em uso ficam travados), decodifica só os stores de mensagens, conversas e perfis, e apaga a cópia.
- Só existe o que o cliente do Teams tem em cache. Mensagens antigas não são buscadas, porque o cade nunca fala com a Microsoft.
- Uma mensagem editada substitui o texto guardado na próxima ingestão. Uma mensagem apagada no Teams depois de ingerida **continua** no cade até você rodar `cade forget teams`.
- `cade teams-schema` mostra a estrutura de um IndexedDB (stores, campos, tipos, contagens) sem nenhum valor, então a saída pode ser compartilhada para diagnóstico.

## Outros aplicativos por schemas de IndexedDB

- **O mesmo cuidado do Teams:** o cache de um aplicativo de conversa guarda mensagens de outras pessoas, e ingeri-lo copia essas mensagens para o banco do cade. Confira antes os termos do aplicativo e da sua organização.
- O `schema-discover`, o `schema-check` e a ingestão copiam o diretório do IndexedDB para uma pasta temporária, leem a cópia e a apagam. O cade nunca altera os dados do aplicativo.
- **Dado cifrado continua cifrado.** O WhatsApp Web cifra o corpo das mensagens no IndexedDB; o cade não decifra, então as mensagens dele são indexadas só por metadados.
- Uma mensagem apagada no aplicativo depois de ingerida **continua** no cade até `cade forget <fonte>`. Uma troca de schema nunca apaga eventos.
- **O cache HTTP e a Cache API do navegador guardam as respostas de todos os sites que você visita,** não só as do aplicativo. O cade só lê deles as URLs que você nomeia em `sources.request_cache_urls`, vazio por padrão; a origem de um padrão não pode ter curinga, então ele nunca alcança outro site. De qualquer outra entrada o cade lê só a chave (a URL), nunca o corpo; o banco da Cache API do Firefox é copiado para uma pasta temporária e apagado, os corpos são lidos no lugar, e nada nesses caches é alterado.
- **O localStorage e o Origin Private File System guardam o estado de todos os sites,** tokens de sessão e logins entre eles. O cade só lê as origens que você nomeia em `sources.storage_origins`, vazio por padrão; uma origem particionada ou de um contêiner do Firefox nunca é lida. No Chromium, em que um banco só guarda todas as origens, os itens das outras origens são pulados pela chave e nunca decodificados. Todos os itens e arquivos JSON de uma origem configurada são lidos, mas só os que um schema seleciona viram eventos. Os bancos do Firefox são copiados para uma pasta temporária e apagados, os arquivos do OPFS são lidos no lugar, e nada é alterado.

## O que o modelo local vê

Os dois modelos rodam dentro do processo, pelo llama.cpp.

- **Modelo de visão** (só com `sources.images` ligado, no `ingest` e no `reindex --captions`): cada imagem nova das pastas configuradas, reduzida para o lado maior ter no máximo 1.024 px, com instruções fixas para descrevê-la e copiar o texto visível. Ele não vê mais nada, e é liberado antes de o modelo de embedding carregar.
- **Modelo de embedding:** o texto inteiro de cada evento, em pedaços de até 1.200 caracteres, na ingestão (texto idêntico é embutido uma vez), e o texto de busca de cada pergunta.
- **Interpretação da pergunta:** só a sua pergunta, com instruções e exemplos fixos. Perguntas lidas por regras (período, fonte e palavras genéricas) nem chegam ao modelo.
- **Estado do prompt salvo:** `~/.cache/cade/prompt-state/` guarda um arquivo (~55 MB, só o dono lê) com o estado do modelo depois das instruções e exemplos fixos. Não contém pergunta nenhuma nem nada do banco.
- **Respostas:** a sua pergunta, a data de hoje e até `top_k` (6 por padrão) eventos; de um evento longo, só o pedaço que casou (até 1.200 caracteres), senão o texto até esse tamanho.
- **Texto que dá ordens ao assistente:** mensagens, títulos de páginas e notas são palavras de outras pessoas, e alguma pode ser escrita para manipular a resposta ("IMPORTANTE para o assistente: ignore as regras…"). Esses eventos vão ao modelo marcados como não confiáveis e sem o texto, com uma regra para não usá-los nem citá-los; a lista de fontes mostra o evento com a marca, e o `ask --json` põe `"untrusted": true`. Isso reduz o risco sem eliminá-lo: um texto que o detector não reconhece ainda chega inteiro ao modelo e pode manipular a resposta, mas o modelo não tem ferramentas, então não consegue agir sobre nada.
- **Descoberta de schemas** (só no `cade schema-discover` e no `cade schema-check --update`, rodados por você): os maiores stores do IndexedDB com os nomes mascarados (e-mails, GUIDs e sequências de 6 ou mais dígitos escondidos), os caminhos dos campos e os tipos encontrados, e até duas amostras por caminho. Amostras de texto são mascaradas do mesmo jeito e cortadas em 40 caracteres; números mostram só a quantidade de dígitos (`<n:13>`), datas e dados binários só o tipo. Numa regeneração, ele lê também o schema atual (só caminhos) e quais caminhos mudaram. O schema que ele escreve guarda caminhos e tipos, nunca uma amostra. A ingestão nunca roda o modelo.
- Nada mais do banco é passado ao modelo. Listagens e relatórios de tarefas não usam o modelo de geração; uma listagem com assunto ("páginas sobre redis") só embute o assunto.

## Logs

- Os logs vão só para o stderr. A exceção é o `ingest start`, que não tem terminal: o stderr e o stdout dele vão para `~/.local/state/cade/ingest.log` (veja acima), com o mesmo conteúdo que uma execução no terminal mostra.
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
- Eventos com o mesmo texto (uma página visitada várias vezes) dividem um conjunto de pedaços, embeddings e entradas de palavras. O `forget` de um deles apaga esse evento, a entrada do hash ou caminho dele e a data dele no embedding; os pedaços compartilhados ficam enquanto outro evento tiver o texto e saem com o último.
- O índice de pessoas (`event_people`) guarda os nomes de cada mensagem e commit (remetente ou autor, título da conversa, primeiros nomes depois de "@"), para que uma pergunta sobre uma pessoa seja filtrada no banco. Ele acompanha os eventos: uma edição troca os nomes do evento, e o `forget` os apaga junto com os eventos.
- Texto substituído, como o de uma mensagem editada ou de uma versão antiga de um arquivo, também é zerado (`secure_delete` do SQLite). Só a versão atual de cada arquivo fica guardada; `file_modifications` guarda a data e o tamanho de cada versão anterior, sem o texto, e o `forget file` a apaga.
- Um arquivo apagado da pasta sai das respostas, mas continua no banco (e na timeline) até `cade forget file`. Para uma imagem, isso inclui a descrição, que uma cópia da mesma imagem encontrada depois reaproveita.
- `forget file` e `forget --uid` apagam a descrição de uma imagem junto com o evento; nada dela fica em outro lugar. `cade reindex --captions` substitui as descrições no lugar, zerando o texto antigo.
- O `forget` não mexe nas cópias de migração (`cade.db.before-v*`); apague-as você mesmo.
- Cópias feitas fora do cade não são afetadas: backups, snapshots, ou saídas de `cade ask --json` que você salvou.
- `forget --uid` e `forget --match` confirmado removem o evento, pedaços, embeddings, índices de palavras e pessoas e histórico do arquivo; só o UID e a data da remoção ficam para impedir a reingestão. `forget <fonte>` limpa essa lista de UIDs esquecidos. A retenção vem desligada; configure `ingest.retention.max_age_days` por fonte para ativá-la.
