package queryplan

import "github.com/chipskein/cade/internal/llm"

// planGrammar allows exactly one JSON shape. Fixed spacing keeps the output
// short; value sets mirror Plan's fields.
const planGrammar = `root ::= "{\"tipo\": " tipo ", \"periodo\": " opcional ", \"fonte\": " fonte ", \"pessoas\": " pessoas ", \"direcao\": " direcao ", \"assunto\": " opcional ", \"status\": " status ", \"entidades\": " pessoas ", \"modalidade\": " modalidade ", \"relacionadas\": " booleano "}"
tipo ::= "\"responder\"" | "\"listar\"" | "\"tarefas\""
status ::= "null" | "\"concluidas\"" | "\"em_andamento\""
opcional ::= "null" | texto
fonte ::= "null" | "\"git\"" | "\"browser\"" | "\"file\"" | "\"teams\""
pessoas ::= "[]" | "[" texto (", " texto){0,3} "]"
direcao ::= "null" | "\"recebidas\"" | "\"enviadas\""
modalidade ::= "null" | "\"imagem\""
booleano ::= "false" | "true"
texto ::= "\"" [^"\\\x00-\x1F]{1,60} "\""`

const planInstructions = `Você converte uma pergunta sobre o histórico de atividade do usuário em filtros JSON. A pergunta pode estar em português ou inglês; os valores fixos do JSON são sempre os listados abaixo.
Preencha SOMENTE o que a pergunta afirma explicitamente; o resto é null ou []. A maioria das perguntas não tem filtros.
- tipo: "tarefas" quando pergunta pelas tarefas/tickets/demandas que trabalhou, concluiu ou que alguém passou; "listar" quando pede os itens em si (as mensagens, os commits, as páginas); "responder" quando pede uma resposta ou explicação.
- periodo: a expressão de tempo completa, copiada da pergunta no idioma dela ("ontem", "semana passada", "últimos 3 dias", "12/08", "yesterday", "last week"), ou null.
- fonte: "teams" (mensagens, chats, conversas), "git" (commits), "browser" (páginas, sites, pesquisas na web), "file" (arquivos), ou null.
- pessoas: nomes de pessoas citados, como escritos. Empresas, clientes, siglas e projetos NÃO são pessoas: vão em assunto.
- direcao (só para mensagens): "recebidas" quando alguém escreveu para o usuário: "me passou", "me pediu", "me perguntou", "me mandou", "recebi", "de X", "da X", "sent me", "asked me", "from X"; "enviadas" quando o usuário escreveu: "mandei", "enviei", "falei para", "pedi para", "para X", "I sent", "I told", "I asked"; null para "com X", "conversa com X", "o que X disse", "what X said" ou quando não se aplica.
- assunto: o tema buscado ("redis", "o deploy da 2.0"), sem pessoas nem datas; null se a pergunta não tem tema.
- status (só para tipo "tarefas"): "concluidas" para "finalizei", "concluí", "terminei", "finished" ou "PR aberto"; "em_andamento" para "em andamento", "pendentes", "não terminei", "in progress"; senão null.
- modalidade: "imagem" quando pede imagens, fotos, prints, capturas de tela, screenshots, images, photos ou pictures; senão null.
- entidades (só com modalidade): quem ou o que aparece na imagem ou é o assunto dela, como escrito ("fotos do João" → ["João"], "prints do projeto X" → ["projeto X"]). Quem mandou ou enviou vai em pessoas, não aqui. Sem modalidade, [].
- relacionadas: true quando pede outros itens ligados aos citados: "quem mais", "o que mais", "outras pessoas", "who else", "what else"; senão false.`

// planExamples teach what a small model gets wrong: no filters for plain
// questions, direction of "me passou", "com X" has no direction, companies
// are not people, who appears in an image is not who sent it. Names differ from real users' to avoid overfitting.
var planExamples = []struct{ question, plan string }{
	{"o que eu fiz relacionado a cache?",
		`{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": "cache", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"Me retorne as mensagens com o Pedro que tive ontem",
		`{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Pedro"], "direcao": null, "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"o que a Carla me passou hoje?",
		`{"tipo": "responder", "periodo": "hoje", "fonte": null, "pessoas": ["Carla"], "direcao": "recebidas", "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"todas as mensagens que mandei para o Rui nos últimos 7 dias",
		`{"tipo": "listar", "periodo": "últimos 7 dias", "fonte": "teams", "pessoas": ["Rui"], "direcao": "enviadas", "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"quais problemas tivemos com a Acme e o ERP ontem?",
		`{"tipo": "responder", "periodo": "ontem", "fonte": null, "pessoas": [], "direcao": null, "assunto": "problemas com a Acme e o ERP", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"liste os commits de 12/08",
		`{"tipo": "listar", "periodo": "12/08", "fonte": "git", "pessoas": [], "direcao": null, "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"que sites visitei semana passada sobre kubernetes?",
		`{"tipo": "listar", "periodo": "semana passada", "fonte": "browser", "pessoas": [], "direcao": null, "assunto": "kubernetes", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"what did Rui send me yesterday about the invoice?",
		`{"tipo": "responder", "periodo": "yesterday", "fonte": null, "pessoas": ["Rui"], "direcao": "recebidas", "assunto": "invoice", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"list the messages I sent to Carla last week",
		`{"tipo": "listar", "periodo": "last week", "fonte": "teams", "pessoas": ["Carla"], "direcao": "enviadas", "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"quais tarefas eu finalizei essa semana?",
		`{"tipo": "tarefas", "periodo": "essa semana", "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": "concluidas", "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"o que eu fiz ontem nas minhas tarefas?",
		`{"tipo": "tarefas", "periodo": "ontem", "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"que demandas o Rui me mandou hoje?",
		`{"tipo": "tarefas", "periodo": "hoje", "fonte": null, "pessoas": ["Rui"], "direcao": "recebidas", "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"quais tickets do cliente Zenite me passaram semana passada?",
		`{"tipo": "tarefas", "periodo": "semana passada", "fonte": null, "pessoas": [], "direcao": "recebidas", "assunto": "Zenite", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"which tasks are still in progress?",
		`{"tipo": "tarefas", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": "em_andamento", "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"chats with Pedro about the release",
		`{"tipo": "responder", "periodo": null, "fonte": "teams", "pessoas": ["Pedro"], "direcao": null, "assunto": "the release", "status": null, "entidades": [], "modalidade": null, "relacionadas": false}`},
	{"fotos da Marta que o Lucas me mandou",
		`{"tipo": "listar", "periodo": null, "fonte": null, "pessoas": ["Lucas"], "direcao": "recebidas", "assunto": null, "status": null, "entidades": ["Marta"], "modalidade": "imagem", "relacionadas": false}`},
	{"qual erro aparecia no print do projeto Atlas?",
		`{"tipo": "responder", "periodo": null, "fonte": null, "pessoas": [], "direcao": null, "assunto": "erro", "status": null, "entidades": ["projeto Atlas"], "modalidade": "imagem", "relacionadas": false}`},
	{"quem mais alterou os arquivos que o Bruno mudou?",
		`{"tipo": "responder", "periodo": null, "fonte": "git", "pessoas": ["Bruno"], "direcao": null, "assunto": null, "status": null, "entidades": [], "modalidade": null, "relacionadas": true}`},
}

func planMessages(question string) []llm.ChatMessage {
	messages := []llm.ChatMessage{{Role: llm.RoleSystem, Content: planInstructions}}
	for _, example := range planExamples {
		messages = append(messages,
			llm.ChatMessage{Role: llm.RoleUser, Content: example.question},
			llm.ChatMessage{Role: llm.RoleAssistant, Content: example.plan})
	}
	return append(messages, llm.ChatMessage{Role: llm.RoleUser, Content: question})
}
