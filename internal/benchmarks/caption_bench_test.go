package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/syntheticimage"
)

const visionProjectorEnv = "CADE_TEST_VISION_PROJECTOR"

// A full-HD screenshot, the common case, scaled to each longer side the
// benchmark compares; 0 sends it unscaled (the model still resizes).
const (
	screenshotWidth  = 1920
	screenshotHeight = 1080
)

var captionSides = []int{512, 768, 1024, 1536, 0}

// denseTerminal fills a screenshot the way a failing test run does.
var denseTerminal = []string{
	"$ go test ./internal/storage/...",
	"--- FAIL: TestSaveEventKeepsChunks (0.02s)",
	"    store_test.go:88: expected 3 chunks, got 0",
	"panic: runtime error: invalid memory address or nil pointer dereference",
	"[signal SIGSEGV: segmentation violation code=0x1 addr=0x18 pc=0x4f2a1c]",
	"goroutine 7 [running]:",
	"github.com/example/app/internal/storage.(*Store).SaveEvent(0x0, {0x7a1e40, 0xc0000a6000})",
	"    /src/app/internal/storage/store.go:142 +0x3c",
	"FAIL    github.com/example/app/internal/storage 0.041s",
	"FAIL",
}

func loadImageDescriber(b *testing.B) *llamacpp.ImageDescriber {
	b.Helper()
	opts := llamacpp.ModelOptions{Path: modelPath(b, generationModelEnv), ContextTokens: config.Defaults().Generation.ContextTokens, GPULayers: -1}
	describer, err := llamacpp.LoadImageDescriber(opts, modelPath(b, visionProjectorEnv))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { describer.Close() })
	return describer
}

func encodedScreenshot(b *testing.B) []byte {
	b.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, syntheticimage.Terminal(denseTerminal, screenshotWidth, screenshotHeight)); err != nil {
		b.Fatal(err)
	}
	return encoded.Bytes()
}

// BenchmarkDescribeImage is ingestion's cost per new image: decoding and
// scaling the file, encoding it, and writing the description, per longer
// side sent to the model.
func BenchmarkDescribeImage(b *testing.B) {
	describer := loadImageDescriber(b)
	raw := encodedScreenshot(b)
	for _, side := range captionSides {
		b.Run(fmt.Sprintf("side=%d", side), func(b *testing.B) {
			limits := imagefile.Limits{MaxPixels: screenshotWidth * screenshotHeight, MaxSide: side}
			describeOnce(b, describer, raw, limits)
			var replyBytes int
			for b.Loop() {
				replyBytes = describeOnce(b, describer, raw, limits)
			}
			b.ReportMetric(float64(replyBytes), "reply_bytes")
			reportMemory(b)
		})
	}
}

func describeOnce(b *testing.B, describer llm.ImageDescriber, raw []byte, limits imagefile.Limits) int {
	b.Helper()
	decoded, err := imagefile.Decode(raw, limits)
	if err != nil {
		b.Fatal(err)
	}
	reply, err := describer.DescribeImage(context.Background(), decoded.Image, imagecaption.Instructions, imagecaption.MaxTokens)
	if err != nil {
		b.Fatal(err)
	}
	return len(reply)
}
