package llamacpp

/*
#include <stdlib.h>
#include "llama.h"
#include "ggml.h"
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

// Each `cade ask` is a new process, so promptCache never helps the planner
// in real use: its ~2 thousand tokens of instructions and examples were
// decoded again for every question. The memory (KV state) after that fixed
// prefix is saved to disk once and loaded by later processes.

const (
	promptStatePrefix    = "prompt-"
	promptStateExtension = ".kvstate"
	// minPersistedPrefix skips prefixes too short to be worth a file.
	minPersistedPrefix = 256
	// promptStateFormat changes when the file name or contents change.
	promptStateFormat = "1"
)

// promptStateStore names the saved states of one loaded model.
type promptStateStore struct {
	dir string
	// identity is everything besides the tokens that changes the saved
	// state: format, llama.cpp version, build, model file, context size and
	// offloaded layers.
	identity string
}

// newPromptStateStore identifies the model file by path, size and
// modification time: hashing its ~2 GB would cost more than the state
// saves. An empty dir disables saving.
func newPromptStateStore(dir string, opts ModelOptions, contextTokens int) (promptStateStore, error) {
	if dir == "" {
		return promptStateStore{}, nil
	}
	absolute, err := filepath.Abs(opts.Path)
	if err != nil {
		return promptStateStore{}, fmt.Errorf("resolve model path %q for the prompt state: %w", opts.Path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return promptStateStore{}, fmt.Errorf("stat model %q for the prompt state: %w", absolute, err)
	}
	identity := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%d|%d", promptStateFormat, C.GoString(C.llama_version()), C.GoString(C.ggml_commit()),
		buildKind, absolute, info.Size(), info.ModTime().UnixNano(), contextTokens, opts.GPULayers)
	return promptStateStore{dir: dir, identity: identity}, nil
}

func (s promptStateStore) enabled() bool {
	return s.dir != ""
}

// path hashes the identity with the prefix tokens, so a changed prompt,
// model or build never loads a stale state.
//
//	path := store.path(tokens) // ~/.cache/cade/prompt-state/prompt-3f2a….kvstate
func (s promptStateStore) path(prefix []C.llama_token) string {
	tokens := make([]int32, len(prefix))
	for i, token := range prefix {
		tokens[i] = int32(token)
	}
	return s.pathForTokens(tokens)
}

func (s promptStateStore) pathForTokens(tokens []int32) string {
	hash := sha256.New()
	hash.Write([]byte(s.identity))
	word := make([]byte, 4)
	for _, token := range tokens {
		binary.LittleEndian.PutUint32(word, uint32(token))
		hash.Write(word)
	}
	name := promptStatePrefix + hex.EncodeToString(hash.Sum(nil))[:32] + promptStateExtension
	return filepath.Join(s.dir, name)
}

// removeOthers deletes every other saved state (older prompts, models,
// builds, and temporary files of interrupted saves): only the planner's
// current prefix is worth keeping, and each file is ~40 MB (Qwen3.5-2B).
func (s promptStateStore) removeOthers(keep string) {
	others, _ := filepath.Glob(filepath.Join(s.dir, promptStatePrefix+"*"))
	for _, other := range others {
		if other != keep {
			os.Remove(other)
		}
	}
}

// restorePrefix fills a fresh context's memory with the saved state of the
// messages before the last one (the planner's instructions and examples),
// saving it first when missing. A failure only costs the speedup.
func (g *Generator) restorePrefix(ctx context.Context, messages []llm.ChatMessage, tokens []C.llama_token) {
	if !g.states.enabled() || len(g.cache.tokens) > 0 || len(messages) < 2 {
		return
	}
	prefix, err := g.sharedPrefix(messages[:len(messages)-1], tokens)
	if err != nil || len(prefix) < minPersistedPrefix {
		return
	}
	path := g.states.path(prefix)
	if g.loadPromptState(path, prefix) {
		return
	}
	if err := g.savePromptState(ctx, path, prefix); err != nil {
		g.logger.Debug("prompt state not saved", "error", err.Error())
	}
}

// sharedPrefix is the part of tokens the earlier messages produce; tokens
// at the boundary may merge differently, so the common prefix is used.
func (g *Generator) sharedPrefix(earlier []llm.ChatMessage, tokens []C.llama_token) ([]C.llama_token, error) {
	prompt, err := applyChatTemplate(g.loaded.model, earlier, false)
	if err != nil {
		return nil, err
	}
	earlierTokens, err := g.loaded.tokenize(prompt, true)
	if err != nil {
		return nil, err
	}
	return tokens[:min(commonPrefix(earlierTokens, tokens), len(tokens)-1)], nil
}

// loadPromptState loads path into the memory; false leaves it empty.
func (g *Generator) loadPromptState(path string, prefix []C.llama_token) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	saved := make([]C.llama_token, len(prefix))
	var count C.size_t
	g.loaded.clearMemory()
	read := C.llama_state_seq_load_file(g.loaded.ctx, cPath, 0, &saved[0], C.size_t(len(saved)), &count)
	if read == 0 || !slices.Equal(saved[:count], prefix) {
		g.loaded.clearMemory()
		g.logger.Debug("prompt state unreadable; decoding the prefix", "path", path, "bytes_read", uint64(read))
		return false
	}
	g.cache.tokens = slices.Clone(prefix)
	g.logger.Debug("prompt state loaded", "path", path, "tokens", len(prefix))
	return true
}

// savePromptState decodes prefix, then writes the memory to a temporary
// file renamed into place, so a concurrent `cade ask` never reads half a
// file. The directory and file are owner-only, like the database.
func (g *Generator) savePromptState(ctx context.Context, path string, prefix []C.llama_token) error {
	if err := g.ingestPrompt(ctx, prefix, llm.GenerationProgress{}); err != nil {
		return err
	}
	if err := os.MkdirAll(g.states.dir, 0o700); err != nil {
		return fmt.Errorf("create prompt state directory %q: %w", g.states.dir, err)
	}
	temporary := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := g.writeMemory(temporary, prefix); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		return fmt.Errorf("move prompt state into %q: %w", path, err)
	}
	g.states.removeOthers(path)
	g.logger.Debug("prompt state saved", "path", path, "tokens", len(prefix))
	return nil
}

func (g *Generator) writeMemory(path string, prefix []C.llama_token) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	if C.llama_state_seq_save_file(g.loaded.ctx, cPath, 0, &prefix[0], C.size_t(len(prefix))) == 0 {
		return fmt.Errorf("save prompt state of %d tokens to %q: llama.cpp wrote nothing", len(prefix), path)
	}
	return os.Chmod(path, 0o600)
}
