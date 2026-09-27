package llamacpp

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestPromptStatePathDependsOnIdentityAndTokens(t *testing.T) {
	store := promptStateStore{dir: "/cache", identity: "a"}
	same := store.pathForTokens([]int32{1, 2, 3})
	if same != store.pathForTokens([]int32{1, 2, 3}) || filepath.Dir(same) != "/cache" || !strings.HasSuffix(same, promptStateExtension) {
		t.Fatalf("expected a stable path under /cache, got %q", same)
	}
	otherTokens := store.pathForTokens([]int32{1, 2, 4})
	otherIdentity := promptStateStore{dir: "/cache", identity: "b"}.pathForTokens([]int32{1, 2, 3})
	if otherTokens == same || otherIdentity == same {
		t.Fatalf("expected new tokens or identity to change the path, got %q %q %q", same, otherTokens, otherIdentity)
	}
}

func TestPromptStateStoreDisabledWithoutDirectory(t *testing.T) {
	store, err := newPromptStateStore("", ModelOptions{Path: "/missing.gguf"}, 4096)
	if err != nil || store.enabled() {
		t.Fatalf("expected a disabled store and no error, got %+v %v", store, err)
	}
}

func TestPromptStateStoreNeedsTheModelFile(t *testing.T) {
	if _, err := newPromptStateStore(t.TempDir(), ModelOptions{Path: "/missing.gguf"}, 4096); err == nil || !strings.Contains(err.Error(), "/missing.gguf") {
		t.Fatalf("expected an error naming the model, got %v", err)
	}
}

func TestPromptStateIdentityFollowsModelFile(t *testing.T) {
	dir, model := t.TempDir(), filepath.Join(t.TempDir(), "model.gguf")
	testcheck.NoError(t, os.WriteFile(model, []byte("v1"), 0o600))
	first, _ := newPromptStateStore(dir, ModelOptions{Path: model}, 4096)
	testcheck.NoError(t, os.WriteFile(model, []byte("v2 longer"), 0o600))
	second, _ := newPromptStateStore(dir, ModelOptions{Path: model}, 4096)
	larger, _ := newPromptStateStore(dir, ModelOptions{Path: model}, 8192)
	if first.identity == second.identity || second.identity == larger.identity {
		t.Fatalf("expected a changed model file or context to change the identity:\n%s\n%s\n%s", first.identity, second.identity, larger.identity)
	}
}

func TestRemoveOthersKeepsOnlyCurrentState(t *testing.T) {
	dir := t.TempDir()
	store := promptStateStore{dir: dir}
	keep := filepath.Join(dir, promptStatePrefix+"new"+promptStateExtension)
	for _, name := range []string{keep, filepath.Join(dir, promptStatePrefix+"old"+promptStateExtension), filepath.Join(dir, promptStatePrefix+"x.kvstate.9.tmp"), filepath.Join(dir, "other.txt")} {
		testcheck.NoError(t, os.WriteFile(name, nil, 0o600))
	}
	store.removeOthers(keep)
	entries, _ := os.ReadDir(dir)
	var left []string
	for _, entry := range entries {
		left = append(left, entry.Name())
	}
	if strings.Join(left, ",") != "other.txt,"+filepath.Base(keep) {
		t.Fatalf("expected only the kept state and unrelated files, got %v", left)
	}
}

// longPrefixMessages builds a prompt whose fixed part (system and example)
// is long enough to be saved, as the planner's is.
func longPrefixMessages(question string) []llm.ChatMessage {
	instructions := strings.Repeat("Responda apenas sim ou não, sem explicar. ", 60)
	return []llm.ChatMessage{
		{Role: llm.RoleSystem, Content: instructions},
		{Role: llm.RoleUser, Content: "o céu é azul?"}, {Role: llm.RoleAssistant, Content: "sim"},
		{Role: llm.RoleUser, Content: question},
	}
}

// A second process (a fresh generator) must load the saved prefix and reply
// exactly as the one that decoded it.
func TestPromptStateIsSavedThenLoadedByAFreshGenerator(t *testing.T) {
	path, dir := modelPathOrSkip(t, generationModelEnv), t.TempDir()
	grammar := `root ::= "sim" | "não"`
	var log bytes.Buffer
	replies := make([]string, 2)
	for i := range replies {
		generator, err := LoadGenerator(ModelOptions{Path: path, ContextTokens: 2048, GPULayers: -1, PromptStateDir: dir,
			Logger: slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))})
		if err != nil {
			t.Fatal(err)
		}
		replies[i], err = generator.GenerateStructured(context.Background(), longPrefixMessages("a água é seca?"), 4, grammar)
		generator.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	states, _ := filepath.Glob(filepath.Join(dir, promptStatePrefix+"*"))
	if replies[0] != replies[1] || len(states) != 1 || !strings.Contains(log.String(), "prompt state saved") || !strings.Contains(log.String(), "prompt state loaded") {
		t.Fatalf("expected one state saved then loaded and equal replies, got %q, files %v, log:\n%s", replies, states, log.String())
	}
}
