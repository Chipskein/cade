package syntheticimage

import "testing"

func TestTerminalHasTheRequestedSizeAndDrawsText(t *testing.T) {
	shot := Terminal([]string{"panic: nil map"}, 300, 120)
	if shot.Bounds().Dx() != 300 || shot.Bounds().Dy() != 120 {
		t.Fatalf("expected 300x120, got %v", shot.Bounds())
	}
	if countColor(shot.Pix, terminalForeground.R) == 0 {
		t.Fatal("expected text pixels in the foreground color")
	}
}

func TestTerminalWithoutLinesIsOnlyBackground(t *testing.T) {
	shot := Terminal(nil, 30, 30)
	if countColor(shot.Pix, terminalForeground.R) != 0 {
		t.Fatal("expected no foreground pixels without lines")
	}
}

// countColor counts pixels whose red channel equals red.
func countColor(pixels []byte, red byte) int {
	count := 0
	for offset := 0; offset < len(pixels); offset += 4 {
		if pixels[offset] == red {
			count++
		}
	}
	return count
}
