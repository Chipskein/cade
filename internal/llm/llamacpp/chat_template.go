package llamacpp

/*
#include <stdlib.h>
#include "llama.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

// applyChatTemplate renders messages with the chat template embedded in the
// GGUF file, ending with the assistant turn opener.
func applyChatTemplate(model *C.struct_llama_model, messages []llm.ChatMessage) (string, error) {
	cMessages, release := toCChatMessages(messages)
	defer release()
	template := C.llama_model_chat_template(model, nil)
	size := C.int32_t(2 * totalContentLength(messages))
	for attempt := 0; attempt < 2; attempt++ {
		buffer := make([]byte, max(size, 256))
		written := C.llama_chat_apply_template(template, cMessages, C.size_t(len(messages)), C.bool(true),
			(*C.char)(unsafe.Pointer(&buffer[0])), C.int32_t(len(buffer)))
		if written < 0 {
			return "", fmt.Errorf("apply chat template to %d messages: unsupported template", len(messages))
		}
		if int(written) <= len(buffer) {
			return string(buffer[:written]), nil
		}
		size = written
	}
	return "", fmt.Errorf("apply chat template to %d messages: output kept growing past %d bytes", len(messages), size)
}

// toCChatMessages copies messages to C memory; call release when done.
func toCChatMessages(messages []llm.ChatMessage) (*C.struct_llama_chat_message, func()) {
	array := (*C.struct_llama_chat_message)(C.malloc(C.size_t(len(messages)) * C.sizeof_struct_llama_chat_message))
	entries := unsafe.Slice(array, len(messages))
	for i, message := range messages {
		entries[i].role = C.CString(string(message.Role))
		entries[i].content = C.CString(message.Content)
	}
	release := func() {
		for i := range entries {
			C.free(unsafe.Pointer(entries[i].role))
			C.free(unsafe.Pointer(entries[i].content))
		}
		C.free(unsafe.Pointer(array))
	}
	return array, release
}

func totalContentLength(messages []llm.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += len(message.Role) + len(message.Content)
	}
	return total
}
