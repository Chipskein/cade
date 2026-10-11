package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
)

func peopleWorld() *fakeWorld {
	world := newFakeWorld()
	world.store.Events = testfakes.PersonCorpus()
	return world
}

func TestForgetPersonListsAndDeletesEverythingAboutThem(t *testing.T) {
	world := peopleWorld()
	code, stdout, stderr := world.run("forget", "--person", "Ana Prado", "--yes")
	if code != 0 || !strings.Contains(stdout, "ana-1to1") || !strings.Contains(stdout, "5 eventos removidos.") {
		t.Fatalf("expected Ana Prado's five events listed and removed, got %d %q %q", code, stdout, stderr)
	}
	var left []string
	for _, ev := range world.store.Events {
		left = append(left, ev.UID)
	}
	if !slices.Equal(left, []string{"caio-group", "me-group", "souza"}) {
		t.Fatalf("expected the group's other messages and the other Ana kept, got %v", left)
	}
}

// "Ana" names two people: nothing is deleted until the name is complete.
func TestForgetPersonRefusesANameOfSeveralPeople(t *testing.T) {
	world := peopleWorld()
	code, _, stderr := world.run("forget", "--person", "Ana", "--yes")
	if code == 0 || !strings.Contains(stderr, "ana prado, ana souza") || len(world.store.Events) != len(testfakes.PersonCorpus()) {
		t.Fatalf("expected a refusal naming both, nothing deleted, got %d %q", code, stderr)
	}
}

func TestForgetPersonOfNobodyFails(t *testing.T) {
	if code, _, stderr := peopleWorld().run("forget", "--person", "Beatriz", "--yes"); code == 0 || !strings.Contains(stderr, "ninguém chamado") {
		t.Fatalf("expected an error, got %d %q", code, stderr)
	}
}

func TestForgetPersonOutsideATerminalNeedsYes(t *testing.T) {
	world := peopleWorld()
	if code, _, _ := world.run("forget", "--person", "Ana Prado"); code == 0 || len(world.store.Events) != len(testfakes.PersonCorpus()) {
		t.Fatalf("expected a refusal without --yes, got %d", code)
	}
}

func TestExportPersonWritesOnlyWhatIsAboutThem(t *testing.T) {
	code, stdout, stderr := peopleWorld().run("export", "--person", "Ana Souza")
	uids := exportedUIDs(t, stdout)
	slices.Sort(uids)
	if code != 0 || !slices.Equal(uids, []string{"caio-mention", "souza"}) || !strings.Contains(stderr, "2 eventos exportados.") {
		t.Fatalf("expected her commit and the @Ana mention, got %d %v %q", code, uids, stderr)
	}
}
