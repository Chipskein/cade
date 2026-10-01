package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
)

var vectorNoun = nounForms{"vetor", "vetores", "vector", "vectors"}

// runCompact rewrites the vector blocks without the positions deletions
// left empty, then compacts the file: `cade compact`. Unlike a reindex it
// computes no embedding, so it takes seconds, not hours of GPU (#65).
func runCompact(ctx context.Context, env commandEnv, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf(env.language.pick("cade compact não recebe argumentos, recebido %q", "cade compact takes no arguments, got %q"), args)
	}
	return env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		before, err := store.VectorSlots(ctx)
		if err != nil {
			return err
		}
		if before.Slots == 0 {
			fmt.Fprintln(env.stdout, env.language.pick("nenhum vetor para compactar.", "no vectors to compact."))
			return nil
		}
		after, err := store.CompactVectors(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintln(env.stdout, compactedLine(before, after, env.language))
		return nil
	})
}

// compactedLine is "posições de vetores: 2048 → 1024, 50% → 0% vazias (1000 vetores)."
func compactedLine(before, after storage.VectorSlots, language Language) string {
	return fmt.Sprintf(language.pick("posições de vetores: %d → %d, %.0f%% → %.0f%% vazias (%s).", "vector positions: %d → %d, %.0f%% → %.0f%% empty (%s)."),
		before.Slots, after.Slots, emptyPercent(before.EmptyShare()), emptyPercent(after.EmptyShare()), language.count(after.Vectors, vectorNoun))
}

func emptyPercent(share float64) float64 {
	return share * 100
}
