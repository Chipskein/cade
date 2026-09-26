package llm

import "testing"

func TestGenerationProgressIgnoresNilCallbacks(t *testing.T) {
	var progress GenerationProgress
	progress.NotifyPrompt(1, 2)
	progress.NotifyToken("x")
}

func TestGenerationProgressForwards(t *testing.T) {
	var done, total int
	var pieces string
	progress := GenerationProgress{
		PromptProcessed: func(d, t int) { done, total = d, t },
		TokenGenerated:  func(piece string) { pieces += piece },
	}
	progress.NotifyPrompt(3, 10)
	progress.NotifyToken("a")
	progress.NotifyToken("b")
	if done != 3 || total != 10 || pieces != "ab" {
		t.Fatalf("unexpected forwarding: %d/%d %q", done, total, pieces)
	}
}
