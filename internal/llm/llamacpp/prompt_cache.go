package llamacpp

/*
#include "llama.h"
*/
import "C"

// promptCache tracks which tokens the context's memory holds, so a prompt
// sharing a prefix with the previous one only decodes what changed. The
// planner sends the same instructions and examples before every question;
// re-reading them made the 36-question plan suite take minutes on CPU.
type promptCache struct {
	tokens []C.llama_token
}

// reuse drops the memory past the prefix tokens shares with what is cached
// and returns how many tokens are already decoded. The last prompt token is
// always decoded again: sampling needs its logits.
func (c *promptCache) reuse(model loadedModel, tokens []C.llama_token) int {
	shared := min(commonPrefix(c.tokens, tokens), len(tokens)-1)
	if shared <= 0 || !model.forgetFrom(shared) {
		model.clearMemory()
		c.tokens = nil
		return 0
	}
	c.tokens = c.tokens[:shared]
	return shared
}

// add records tokens just decoded into the memory.
func (c *promptCache) add(tokens []C.llama_token) {
	c.tokens = append(c.tokens, tokens...)
}

// reset marks the memory as unknown after a failed decode; the next reuse
// clears it.
func (c *promptCache) reset() {
	c.tokens = nil
}

// commonPrefix is the length of the longest shared prefix of a and b.
func commonPrefix[T comparable](a, b []T) int {
	shared := 0
	for shared < min(len(a), len(b)) && a[shared] == b[shared] {
		shared++
	}
	return shared
}
