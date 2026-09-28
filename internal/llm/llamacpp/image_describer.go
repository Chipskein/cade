package llamacpp

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/llama.cpp/tools/mtmd
#include <stdlib.h>
#include <string.h>
#include "mtmd.h"
#include "mtmd-helper.h"

static int32_t cade_tokenize_with_image(mtmd_context * ctx, mtmd_input_chunks * chunks, const char * prompt, const mtmd_bitmap * bitmap) {
	mtmd_input_text text = { prompt, strlen(prompt), true, true };
	const mtmd_bitmap * bitmaps[1] = { bitmap };
	return mtmd_tokenize(ctx, chunks, &text, bitmaps, 1);
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/chipskein/cade/internal/llm"
)

// rgbBytesPerPixel is the layout of llm.RGBImage, which mtmd_bitmap_init
// expects too.
const rgbBytesPerPixel = 3

// ImageDescriber is a generator with its vision projector: only `ingest`
// loads it, and frees it before the embedder loads, so the two never add
// up in memory (phase 19).
type ImageDescriber struct {
	generator *Generator
	projector *VisionProjector
}

var _ llm.ImageDescriber = (*ImageDescriber)(nil)

// LoadImageDescriber loads the chat model and its mmproj; both must come
// from the same release. Call Close to release them.
//
//	describer, err := llamacpp.LoadImageDescriber(llamacpp.ModelOptions{Path: "Qwen3.5-2B-Q4_K_M.gguf", ContextTokens: 4096}, "mmproj-Qwen3.5-2B-F16.gguf")
func LoadImageDescriber(opts ModelOptions, projectorPath string) (*ImageDescriber, error) {
	generator, err := LoadGenerator(opts)
	if err != nil {
		return nil, err
	}
	projector, err := loadImageProjector(generator, projectorPath, opts.GPULayers != 0)
	if err != nil {
		generator.Close()
		return nil, err
	}
	return &ImageDescriber{generator: generator, projector: projector}, nil
}

func loadImageProjector(generator *Generator, path string, useGPU bool) (*VisionProjector, error) {
	projector, err := LoadVisionProjector(generator, path, useGPU)
	if err != nil {
		return nil, err
	}
	if !projector.SupportsImages() {
		projector.Close()
		return nil, fmt.Errorf("vision projector %q encodes no images; expected the image mmproj of the generation model", path)
	}
	return projector, nil
}

// DescribeImage replies to instructions about image, sampling greedily.
// The image goes before the instructions in one user turn.
func (d *ImageDescriber) DescribeImage(ctx context.Context, image llm.RGBImage, instructions string, maxTokens int) (string, error) {
	if err := checkRGBImage(image); err != nil {
		return "", err
	}
	g := d.generator
	g.mu.Lock()
	defer g.mu.Unlock()
	// The memory now holds image positions the text prompt cache knows
	// nothing about; the next Generate must start from scratch.
	defer g.cache.reset()
	chunks, err := d.tokenizeWithImage(image, instructions)
	if err != nil {
		return "", err
	}
	defer C.mtmd_input_chunks_free(chunks)
	if err := d.evaluatePrompt(ctx, chunks, maxTokens); err != nil {
		return "", err
	}
	sampler := C.llama_sampler_init_greedy()
	defer C.llama_sampler_free(sampler)
	return g.sampleReply(ctx, sampler, maxTokens, llm.GenerationProgress{})
}

func checkRGBImage(image llm.RGBImage) error {
	if image.Width <= 0 || image.Height <= 0 || len(image.Pixels) != image.Width*image.Height*rgbBytesPerPixel {
		return fmt.Errorf("image of %dx%d with %d bytes: expected a positive size and %d bytes per pixel",
			image.Width, image.Height, len(image.Pixels), rgbBytesPerPixel)
	}
	return nil
}

// tokenizeWithImage renders the chat prompt with the media marker, which
// mtmd replaces by the image's tokens. The caller frees the chunks.
func (d *ImageDescriber) tokenizeWithImage(image llm.RGBImage, instructions string) (*C.mtmd_input_chunks, error) {
	turn := llm.ChatMessage{Role: llm.RoleUser, Content: C.GoString(C.mtmd_default_marker()) + "\n" + instructions}
	prompt, err := applyChatTemplate(d.generator.loaded.model, []llm.ChatMessage{turn}, true)
	if err != nil {
		return nil, err
	}
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	// mtmd_bitmap_init copies the pixels, so no Go memory is kept by C.
	bitmap := C.mtmd_bitmap_init(C.uint32_t(image.Width), C.uint32_t(image.Height), (*C.uchar)(unsafe.Pointer(&image.Pixels[0])))
	defer C.mtmd_bitmap_free(bitmap)
	chunks := C.mtmd_input_chunks_init()
	if status := C.cade_tokenize_with_image(d.projector.ctx, chunks, cPrompt, bitmap); status != 0 {
		C.mtmd_input_chunks_free(chunks)
		return nil, fmt.Errorf("tokenize prompt with a %dx%d image: mtmd status %d (1 = marker count mismatch, 2 = image preprocessing failed)",
			image.Width, image.Height, status)
	}
	return chunks, nil
}

// evaluatePrompt encodes the image and decodes the text around it from
// an empty memory. The prompt ends with text, so the reply's positions
// continue from the last text token (M-RoPE models such as Qwen3.5 give
// an image fewer positions than tokens).
func (d *ImageDescriber) evaluatePrompt(ctx context.Context, chunks *C.mtmd_input_chunks, maxTokens int) error {
	g := d.generator
	if err := g.checkFits(int(C.mtmd_helper_get_n_tokens(chunks)), maxTokens); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	g.loaded.clearMemory()
	var nextPosition C.llama_pos
	status := C.mtmd_helper_eval_chunks(d.projector.ctx, g.loaded.ctx, chunks, 0, 0, C.int32_t(g.batchSize), C.bool(true), &nextPosition)
	if status != 0 {
		return fmt.Errorf("encode image prompt of %d tokens: mtmd status %d", int(C.mtmd_helper_get_n_tokens(chunks)), status)
	}
	return nil
}

// Close frees the projector, then the generator it was made for.
func (d *ImageDescriber) Close() error {
	d.projector.Close()
	return d.generator.Close()
}
