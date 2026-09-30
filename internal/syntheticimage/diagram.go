package syntheticimage

import (
	"image"
	"image/draw"

	"golang.org/x/image/font/basicfont"
)

// Diagram layout, in unscaled pixels: boxes in a row, joined by arrows.
const (
	boxPadding  = 8
	boxHeight   = 32
	arrowLength = 28
	arrowHead   = 5
	borderWidth = 1
)

// Diagram draws labels (ASCII) as boxes left to right, each joined to the
// next by an arrow, under a title: an architecture sketch.
//
//	shot := syntheticimage.Diagram("Fluxo de pedidos", []string{"API", "Fila", "Worker"}, 1280, 480)
func Diagram(title string, labels []string, width, height int) *image.RGBA {
	small := image.NewRGBA(image.Rect(0, 0, width/glyphScale, height/glyphScale))
	draw.Draw(small, small.Bounds(), image.NewUniform(pagePalette.background), image.Point{}, draw.Src)
	face := basicfont.Face7x13
	margin := face.Advance * marginCells
	drawText(small, title, image.Pt(margin, margin+face.Ascent), pagePalette.foreground)
	top := margin + 2*face.Height + lineSpacing
	left := margin
	for i, label := range labels {
		left = drawBox(small, label, left, top)
		if i < len(labels)-1 {
			left = drawArrow(small, left, top+boxHeight/2)
		}
	}
	return enlarged(small, width, height, pagePalette.background)
}

// drawBox frames label at (left, top) and returns the box's right edge.
func drawBox(canvas *image.RGBA, label string, left, top int) int {
	face := basicfont.Face7x13
	right := left + len(label)*face.Advance + 2*boxPadding
	frame := image.Rect(left, top, right, top+boxHeight)
	fill(canvas, image.Rect(frame.Min.X, frame.Min.Y, frame.Max.X, frame.Min.Y+borderWidth))
	fill(canvas, image.Rect(frame.Min.X, frame.Max.Y-borderWidth, frame.Max.X, frame.Max.Y))
	fill(canvas, image.Rect(frame.Min.X, frame.Min.Y, frame.Min.X+borderWidth, frame.Max.Y))
	fill(canvas, image.Rect(frame.Max.X-borderWidth, frame.Min.Y, frame.Max.X, frame.Max.Y))
	baseline := top + (boxHeight+face.Ascent)/2
	drawText(canvas, label, image.Pt(left+boxPadding, baseline), pagePalette.foreground)
	return right
}

// drawArrow draws a right-pointing arrow from left at height y and returns
// where it ends.
func drawArrow(canvas *image.RGBA, left, y int) int {
	right := left + arrowLength
	fill(canvas, image.Rect(left, y, right, y+borderWidth))
	for step := 1; step <= arrowHead; step++ {
		fill(canvas, image.Rect(right-step, y-step, right-step+borderWidth, y+step+borderWidth))
	}
	return right
}

func fill(canvas *image.RGBA, area image.Rectangle) {
	draw.Draw(canvas, area, image.NewUniform(pagePalette.foreground), image.Point{}, draw.Src)
}
