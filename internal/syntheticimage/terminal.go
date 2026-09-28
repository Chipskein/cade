// Package syntheticimage draws screenshots with known text, for the tests,
// benchmarks and evaluation fixtures of image descriptions (phase 19): no
// real screenshot, and so no real name or secret, is ever needed.
package syntheticimage

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Text scaling: basicfont's 7x13 glyphs are too small for a vision model to
// read once the image is resized, so each glyph pixel becomes a block.
const (
	glyphScale  = 3
	lineSpacing = 4 // unscaled pixels between lines
	marginCells = 2 // unscaled glyph widths of margin
)

// Terminal colors: light text on a dark background, like most terminals.
var (
	terminalBackground = color.RGBA{R: 30, G: 30, B: 30, A: 255}
	terminalForeground = color.RGBA{R: 230, G: 230, B: 230, A: 255}
)

// Terminal draws lines (ASCII) as a terminal screenshot of width x height
// pixels; lines that do not fit are cut.
//
//	shot := syntheticimage.Terminal([]string{"$ go test", "FAIL"}, 1024, 640)
func Terminal(lines []string, width, height int) *image.RGBA {
	small := image.NewRGBA(image.Rect(0, 0, width/glyphScale, height/glyphScale))
	draw.Draw(small, small.Bounds(), image.NewUniform(terminalBackground), image.Point{}, draw.Src)
	drawLines(small, lines)
	return enlarged(small, width, height)
}

func drawLines(canvas *image.RGBA, lines []string) {
	face := basicfont.Face7x13
	margin := face.Advance * marginCells
	drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(terminalForeground), Face: face}
	for i, line := range lines {
		baseline := margin + face.Ascent + i*(face.Height+lineSpacing)
		drawer.Dot = fixed.P(margin, baseline)
		drawer.DrawString(line)
	}
}

// enlarged repeats each pixel glyphScale times, keeping glyph edges sharp.
func enlarged(small *image.RGBA, width, height int) *image.RGBA {
	large := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(large, large.Bounds(), image.NewUniform(terminalBackground), image.Point{}, draw.Src)
	for y := range small.Bounds().Dy() * glyphScale {
		for x := range small.Bounds().Dx() * glyphScale {
			large.SetRGBA(x, y, small.RGBAAt(x/glyphScale, y/glyphScale))
		}
	}
	return large
}
