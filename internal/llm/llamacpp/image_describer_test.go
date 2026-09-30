package llamacpp

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/syntheticimage"
)

const describerContextTokens = 4096

var testImageLimits = imagefile.Limits{MaxPixels: 1 << 24, MaxSide: 1024}

func TestCheckRGBImage(t *testing.T) {
	valid := llm.RGBImage{Width: 2, Height: 1, Pixels: make([]byte, 6)}
	if err := checkRGBImage(valid); err != nil {
		t.Fatalf("expected 2x1 with 6 bytes to be valid: %v", err)
	}
	for _, invalid := range []llm.RGBImage{{Width: 2, Height: 1, Pixels: make([]byte, 5)}, {Width: 0, Height: 1}, {Width: 1, Height: -1, Pixels: make([]byte, 3)}} {
		if err := checkRGBImage(invalid); err == nil {
			t.Errorf("expected %dx%d with %d bytes to be rejected", invalid.Width, invalid.Height, len(invalid.Pixels))
		}
	}
}

func loadTestDescriber(t *testing.T) *ImageDescriber {
	t.Helper()
	opts := ModelOptions{Path: modelPathOrSkip(t, generationModelEnv), ContextTokens: describerContextTokens, GPULayers: -1}
	describer, err := LoadImageDescriber(opts, modelPathOrSkip(t, visionProjectorEnv))
	if err != nil {
		t.Fatalf("load image describer: %v", err)
	}
	t.Cleanup(func() { describer.Close() })
	return describer
}

func terminalScreenshot(t *testing.T, lines []string) llm.RGBImage {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, syntheticimage.Terminal(lines, 1024, 480)); err != nil {
		t.Fatalf("encode screenshot: %v", err)
	}
	decoded, err := imagefile.Decode(encoded.Bytes(), testImageLimits)
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	return decoded.Image
}

func TestImageDescriberTranscribesTerminalText(t *testing.T) {
	describer := loadTestDescriber(t)
	shot := terminalScreenshot(t, []string{"$ go run ./cmd/server", "panic: assignment to entry in nil map"})
	reply, err := describer.DescribeImage(context.Background(), shot, imagecaption.Instructions, imagecaption.MaxTokens)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	caption := imagecaption.ParseCaption(reply)
	if !strings.Contains(strings.ToLower(caption.VisibleText), "nil map") {
		t.Fatalf("expected the panic transcribed, got %+v (reply %q)", caption, reply)
	}
}

// Greedy sampling is what lets a stored description stand for an image:
// describing it again must give the same text, even after text generation
// used the same context.
func TestImageDescriberIsDeterministicAcrossTextGeneration(t *testing.T) {
	describer := loadTestDescriber(t)
	shot := terminalScreenshot(t, []string{"ERROR 504 Gateway Timeout"})
	first, err := describer.DescribeImage(context.Background(), shot, imagecaption.Instructions, imagecaption.MaxTokens)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if _, err := describer.generator.Generate(context.Background(), capitalQuestion, 8, llm.GenerationProgress{}); err != nil {
		t.Fatalf("generate between descriptions: %v", err)
	}
	second, err := describer.DescribeImage(context.Background(), shot, imagecaption.Instructions, imagecaption.MaxTokens)
	if err != nil || first != second {
		t.Fatalf("expected the same description twice, got %q and %q (err %v)", first, second, err)
	}
}

func TestImageDescriberRejectsMalformedPixels(t *testing.T) {
	describer := loadTestDescriber(t)
	_, err := describer.DescribeImage(context.Background(), llm.RGBImage{Width: 4, Height: 4, Pixels: make([]byte, 3)}, imagecaption.Instructions, 8)
	if err == nil {
		t.Fatal("expected an error for 3 bytes of a 4x4 image")
	}
}

func TestLoadImageDescriberRejectsMissingProjector(t *testing.T) {
	opts := ModelOptions{Path: modelPathOrSkip(t, generationModelEnv), ContextTokens: describerContextTokens}
	if _, err := LoadImageDescriber(opts, "/nonexistent/mmproj.gguf"); err == nil {
		t.Fatal("expected an error for a missing projector")
	}
}
