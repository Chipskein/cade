package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/storage"
)

// portugueseLabels are words only the Portuguese labels use; the fixtures'
// content (task titles, messages) avoids them.
var portugueseLabels = []string{
	"evento", "Evento", "tarefa", "Tarefa", "Entendi", "Fontes", "Nenhum", "concluída", "andamento", "Não encontrei",
	"pessoas:", "assunto:", "Filtrando", "Consultadas", "Citadas", "aberto", "provável", "suas", "recebidas",
	"novos", "atualizados", "Carregando", "Buscando", "Gerando", "erro:",
}

func englishWorld() *fakeWorld {
	world := newFakeWorld()
	world.language = English
	return world
}

func requireEnglish(t *testing.T, outputs ...string) {
	t.Helper()
	for _, output := range outputs {
		for _, label := range portugueseLabels {
			if strings.Contains(output, label) {
				t.Errorf("Portuguese label %q in English output:\n%s", label, output)
			}
		}
	}
}

func TestTimelineInEnglish(t *testing.T) {
	world := englishWorld()
	_, ingested, _ := world.run("ingest", "git")
	code, stdout, stderr := world.run("timeline", "hoje")
	if code != 0 || !strings.Contains(stdout, "Timeline of 2026-09-26 — 1 event\n") || !strings.Contains(ingested, "1 new, 0 updated, 0 already stored (1 read)") {
		t.Fatalf("expected an English timeline, got %d %q %q", code, stdout, ingested)
	}
	_, empty, _ := world.run("timeline", "2026-01-01")
	requireEnglish(t, stdout, stderr, ingested, empty)
}

func TestTasksInEnglish(t *testing.T) {
	world := englishWorld()
	finishedAndOpenTasks(world)
	code, stdout, stderr := world.run("tasks", "ontem")
	if code != 0 || !strings.Contains(stdout, "finished      14/162  Ajuste de CEP") || !strings.Contains(stdout, "PR acme/api#45 opened 09:21") {
		t.Fatalf("expected an English task report, got %d:\n%s", code, stdout)
	}
	_, none, _ := world.run("tasks", "2026-01-01")
	requireEnglish(t, stdout, stderr, none)
}

func TestAskAnswerInEnglish(t *testing.T) {
	world := englishWorld()
	world.store.SearchResults = []storage.ScoredEvent{{Event: sampleCommit, Distance: 0.1}}
	world.generator.Reply = "You fixed the login [1]."
	code, stdout, stderr := world.run("ask", "what did I do?")
	if code != 0 || !strings.Contains(stdout, "Cited sources:\n  [1] [git]") || !strings.Contains(stderr, "Understood: answer") {
		t.Fatalf("expected an English answer, got %d %q %q", code, stdout, stderr)
	}
	_, notFound, _ := englishWorld().run("ask", "cake recipe?")
	requireEnglish(t, stdout, stderr, notFound)
}

func TestAskListingInEnglish(t *testing.T) {
	world := englishWorld()
	dayOfTeamsMessages(world)
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Ana"], "direcao": "recebidas", "assunto": null}`
	code, stdout, stderr := world.run("ask", "messages Ana sent me yesterday")
	if code != 0 || !strings.Contains(stderr, "Understood: list · teams · 2026-09-25 · people: Ana · received") || !strings.Contains(stderr, "Filtering by person: ana") {
		t.Fatalf("expected an English listing, got %d %q %q", code, stdout, stderr)
	}
	requireEnglish(t, stdout, stderr)
}

func TestAskTasksInEnglish(t *testing.T) {
	world := englishWorld()
	finishedAndOpenTasks(world)
	world.generator.StructuredReply = `{"tipo": "tarefas", "periodo": "ontem", "fonte": null, "pessoas": [], "direcao": null, "assunto": null, "status": "concluidas"}`
	code, stdout, stderr := world.run("ask", "which tasks did I finish yesterday?")
	if code != 0 || !strings.Contains(stderr, "Understood: tasks · 2026-09-25 · finished") {
		t.Fatalf("expected an English task answer, got %d %q %q", code, stdout, stderr)
	}
	requireEnglish(t, stdout, stderr)
}

func TestErrorsInEnglish(t *testing.T) {
	_, _, stderr := englishWorld().run("ask")
	if !strings.HasPrefix(stderr, "error: type the question") {
		t.Fatalf("expected an English error, got %q", stderr)
	}
}

func TestUILanguageOverridesLocale(t *testing.T) {
	world := newFakeWorld()
	world.cfg.UI.Language = "en"
	if _, stdout, _ := world.run("timeline", "2026-01-01"); !strings.Contains(stdout, "No events on 2026-01-01") {
		t.Fatalf("expected ui.language en over a Portuguese locale, got %q", stdout)
	}
	world = englishWorld()
	world.cfg.UI.Language = "pt"
	if _, stdout, _ := world.run("help"); !strings.Contains(stdout, "Uso:") {
		t.Fatalf("expected ui.language pt over an English locale, got %q", stdout)
	}
}

func TestLanguageFromSetting(t *testing.T) {
	cases := []struct {
		setting    string
		fromLocale Language
		want       Language
	}{{"auto", English, English}, {"auto", Portuguese, Portuguese}, {"pt", English, Portuguese}, {"en", Portuguese, English}}
	for _, c := range cases {
		if got := languageFromSetting(c.setting, c.fromLocale); got != c.want {
			t.Errorf("languageFromSetting(%q, %d) = %d, want %d", c.setting, c.fromLocale, got, c.want)
		}
	}
}
