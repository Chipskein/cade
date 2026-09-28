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
// read once the image is resized, so each glyph pixel becomes a block. The
// font is ASCII only: fixture text is written without accents.
const (
	glyphScale  = 3
	lineSpacing = 4 // unscaled pixels between lines
	marginCells = 2 // unscaled glyph widths of margin
)

// palette is a screenshot's background and text colors.
type palette struct {
	background color.RGBA
	foreground color.RGBA
}

var (
	// terminalPalette: light text on a dark background, like most terminals.
	terminalPalette = palette{background: color.RGBA{R: 30, G: 30, B: 30, A: 255}, foreground: color.RGBA{R: 230, G: 230, B: 230, A: 255}}
	// pagePalette: dark text on white, like a web page or document.
	pagePalette = palette{background: color.RGBA{R: 255, G: 255, B: 255, A: 255}, foreground: color.RGBA{R: 20, G: 20, B: 20, A: 255}}
)

// Terminal draws lines (ASCII) as a terminal screenshot of width x height
// pixels; lines that do not fit are cut.
//
//	shot := syntheticimage.Terminal([]string{"$ go test", "FAIL"}, 1024, 640)
func Terminal(lines []string, width, height int) *image.RGBA {
	return textScreenshot(lines, width, height, terminalPalette)
}

// Page draws lines (ASCII) as a light page, such as a browser tab or a
// document.
//
//	shot := syntheticimage.Page([]string{"502 Bad Gateway", "nginx"}, 1024, 640)
func Page(lines []string, width, height int) *image.RGBA {
	return textScreenshot(lines, width, height, pagePalette)
}

func textScreenshot(lines []string, width, height int, colors palette) *image.RGBA {
	small := image.NewRGBA(image.Rect(0, 0, width/glyphScale, height/glyphScale))
	draw.Draw(small, small.Bounds(), image.NewUniform(colors.background), image.Point{}, draw.Src)
	drawLines(small, lines, colors.foreground)
	return enlarged(small, width, height, colors.background)
}

func drawLines(canvas *image.RGBA, lines []string, foreground color.RGBA) {
	face := basicfont.Face7x13
	margin := face.Advance * marginCells
	for i, line := range lines {
		drawText(canvas, line, image.Pt(margin, margin+face.Ascent+i*(face.Height+lineSpacing)), foreground)
	}
}

// drawText writes text with its baseline starting at dot.
func drawText(canvas *image.RGBA, text string, dot image.Point, foreground color.RGBA) {
	drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(foreground), Face: basicfont.Face7x13, Dot: fixed.P(dot.X, dot.Y)}
	drawer.DrawString(text)
}

// enlarged repeats each pixel glyphScale times, keeping glyph edges sharp.
func enlarged(small *image.RGBA, width, height int, background color.RGBA) *image.RGBA {
	large := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(large, large.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	for y := range small.Bounds().Dy() * glyphScale {
		for x := range small.Bounds().Dx() * glyphScale {
			large.SetRGBA(x, y, small.RGBAAt(x/glyphScale, y/glyphScale))
		}
	}
	return large
}
