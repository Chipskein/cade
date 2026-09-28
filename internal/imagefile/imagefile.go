// Package imagefile decodes the image files cade describes (png, jpeg,
// webp) into RGB pixels for the vision model (phase 19). It wraps
// golang.org/x/image so no other package imports it.
package imagefile

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // registers the jpeg decoder
	_ "image/png"  // registers the png decoder
	"path/filepath"
	"slices"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the webp decoder

	"github.com/chipskein/cade/internal/llm"
)

// Extension is a file extension (lower case, with the dot) cade describes.
type Extension string

const (
	ExtensionPNG  Extension = ".png"
	ExtensionJPG  Extension = ".jpg"
	ExtensionJPEG Extension = ".jpeg"
	ExtensionWebP Extension = ".webp"
)

// DescribedExtensions are the image types sent to the vision model.
var DescribedExtensions = []Extension{ExtensionPNG, ExtensionJPG, ExtensionJPEG, ExtensionWebP}

// IsDescribedImage reports whether path has one of DescribedExtensions,
// ignoring case ("Print.PNG" counts).
//
//	if imagefile.IsDescribedImage("/notes/erro.png") { ... }
func IsDescribedImage(path string) bool {
	return slices.Contains(DescribedExtensions, Extension(strings.ToLower(filepath.Ext(path))))
}

// Limits bound what Decode accepts and returns.
type Limits struct {
	// MaxPixels rejects an image before decoding its pixels: a small file
	// can declare 50 000 × 50 000 and exhaust memory ("decompression bomb").
	MaxPixels int
	// MaxSide scales the longer side down to this size; the vision model
	// resizes anyway, and encoding a smaller image is faster.
	MaxSide int
}

// Dimensions are an image's size as stored in its file.
type Dimensions struct {
	Width  int
	Height int
}

// Decoded is an image ready for the vision model, with the size of the
// original file (the pixels may be scaled down).
type Decoded struct {
	Original Dimensions
	Image    llm.RGBImage
}

// Decode reads raw (a png, jpeg or webp file) within limits.
//
//	decoded, err := imagefile.Decode(raw, imagefile.Limits{MaxPixels: 50_000_000, MaxSide: 1024})
func Decode(raw []byte, limits Limits) (Decoded, error) {
	original, err := checkedDimensions(raw, limits)
	if err != nil {
		return Decoded{}, err
	}
	picture, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Decoded{}, fmt.Errorf("decode %d-byte image of %dx%d: %w", len(raw), original.Width, original.Height, err)
	}
	return Decoded{Original: original, Image: toRGB(scaledDown(picture, limits.MaxSide))}, nil
}

func checkedDimensions(raw []byte, limits Limits) (Dimensions, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Dimensions{}, fmt.Errorf("read header of %d-byte image: expected png, jpeg or webp: %w", len(raw), err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return Dimensions{}, fmt.Errorf("%s image declares %dx%d pixels, expected a positive size", format, config.Width, config.Height)
	}
	if config.Width*config.Height > limits.MaxPixels {
		return Dimensions{}, fmt.Errorf("%s image of %dx%d pixels exceeds the limit of %d pixels", format, config.Width, config.Height, limits.MaxPixels)
	}
	return Dimensions{Width: config.Width, Height: config.Height}, nil
}

// scaledDown keeps the aspect ratio; an image within maxSide is unchanged.
func scaledDown(picture image.Image, maxSide int) image.Image {
	bounds := picture.Bounds()
	longer := max(bounds.Dx(), bounds.Dy())
	if maxSide <= 0 || longer <= maxSide {
		return picture
	}
	width := max(1, bounds.Dx()*maxSide/longer)
	height := max(1, bounds.Dy()*maxSide/longer)
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), picture, bounds, draw.Src, nil)
	return scaled
}

// Bytes per pixel in llm.RGBImage and in image.RGBA.Pix.
const (
	rgbChannels  = 3
	rgbaChannels = 4
)

// toRGB drops alpha after compositing over white: a transparent screenshot
// background would otherwise turn black and hide dark text.
func toRGB(picture image.Image) llm.RGBImage {
	bounds := picture.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), picture, bounds.Min, draw.Over)
	pixels := make([]byte, 0, bounds.Dx()*bounds.Dy()*rgbChannels)
	for offset := 0; offset < len(canvas.Pix); offset += rgbaChannels {
		pixels = append(pixels, canvas.Pix[offset:offset+rgbChannels]...)
	}
	return llm.RGBImage{Width: bounds.Dx(), Height: bounds.Dy(), Pixels: pixels}
}
