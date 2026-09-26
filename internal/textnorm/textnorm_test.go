package textnorm

import "testing"

func TestFold(t *testing.T) {
	if got := Fold("Última AÇÃO no mês"); got != "ultima acao no mes" {
		t.Fatalf("unexpected folding %q", got)
	}
}
