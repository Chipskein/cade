package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/storage"
)

func TestCompactReportsThePositionsBeforeAndAfter(t *testing.T) {
	world := newFakeWorld()
	world.store.Slots = storage.VectorSlots{Slots: 2048, Vectors: 1000}
	world.store.CompactedSlots = storage.VectorSlots{Slots: 1024, Vectors: 1000}
	code, stdout, stderr := world.run("compact")
	want := "posições de vetores: 2048 → 1024, 51% → 2% vazias (1000 vetores).\n"
	if code != 0 || stdout != want || world.store.Compactions != 1 {
		t.Fatalf("expected one compaction and %q, got %d %q %q (%d compactions)", want, code, stdout, stderr, world.store.Compactions)
	}
}

func TestCompactWithoutVectorsDoesNotRewrite(t *testing.T) {
	world := newFakeWorld()
	code, stdout, _ := world.run("compact")
	if code != 0 || !strings.Contains(stdout, "nenhum vetor") || world.store.Compactions != 0 {
		t.Fatalf("expected nothing compacted, got %d %q (%d compactions)", code, stdout, world.store.Compactions)
	}
}

func TestCompactRejectsArguments(t *testing.T) {
	world := newFakeWorld()
	code, _, stderr := world.run("compact", "file")
	if code == 0 || !strings.Contains(stderr, `"file"`) || world.store.Compactions != 0 {
		t.Fatalf("expected an error naming the argument, got %d %q", code, stderr)
	}
}

func TestCompactInEnglish(t *testing.T) {
	world := englishWorld()
	world.store.Slots = storage.VectorSlots{Slots: 1024, Vectors: 1}
	world.store.CompactedSlots = world.store.Slots
	code, stdout, stderr := world.run("compact")
	requireEnglish(t, stdout, stderr)
	if code != 0 || !strings.Contains(stdout, "vector positions: 1024 → 1024") || !strings.Contains(stdout, "(1 vector)") {
		t.Fatalf("expected an English summary, got %d %q", code, stdout)
	}
}
