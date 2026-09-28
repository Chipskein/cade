package imagefile

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

var generousLimits = Limits{MaxPixels: 1 << 20, MaxSide: 1 << 10}

func encodedPNG(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

func filled(width, height int, fill color.Color) *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, fill)
		}
	}
	return picture
}

func TestIsDescribedImage(t *testing.T) {
	for path, want := range map[string]bool{"/a/erro.png": true, "/a/Foto.JPG": true, "/a/b.jpeg": true, "/a/c.webp": true, "/a/d.gif": false, "/a/png": false} {
		if got := IsDescribedImage(path); got != want {
			t.Errorf("IsDescribedImage(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestDecodeKeepsSmallImageAsRGB(t *testing.T) {
	decoded, err := Decode(encodedPNG(t, filled(3, 2, color.NRGBA{R: 200, G: 10, B: 20, A: 255})), generousLimits)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Original != (Dimensions{Width: 3, Height: 2}) || decoded.Image.Width != 3 || decoded.Image.Height != 2 {
		t.Fatalf("expected 3x2 kept, got original %+v image %dx%d", decoded.Original, decoded.Image.Width, decoded.Image.Height)
	}
	if len(decoded.Image.Pixels) != 3*2*rgbChannels || !bytes.Equal(decoded.Image.Pixels[:3], []byte{200, 10, 20}) {
		t.Fatalf("expected RGB bytes 200,10,20 per pixel, got %v", decoded.Image.Pixels)
	}
}

func TestDecodeScalesTheLongerSideDown(t *testing.T) {
	decoded, err := Decode(encodedPNG(t, filled(400, 100, color.White)), Limits{MaxPixels: 1 << 20, MaxSide: 200})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Image.Width != 200 || decoded.Image.Height != 50 || decoded.Original.Width != 400 {
		t.Fatalf("expected 400x100 scaled to 200x50 keeping the original size, got %dx%d from %+v", decoded.Image.Width, decoded.Image.Height, decoded.Original)
	}
}

// A transparent screenshot background must not turn black: dark text on
// it would vanish.
func TestDecodeCompositesTransparencyOverWhite(t *testing.T) {
	decoded, err := Decode(encodedPNG(t, filled(1, 1, color.NRGBA{})), generousLimits)
	if err != nil || !bytes.Equal(decoded.Image.Pixels, []byte{255, 255, 255}) {
		t.Fatalf("expected a transparent pixel to become white, got %v (err %v)", decoded.Image.Pixels, err)
	}
}

func TestDecodeRejectsTooManyPixelsBeforeDecoding(t *testing.T) {
	_, err := Decode(encodedPNG(t, filled(100, 100, color.White)), Limits{MaxPixels: 9999})
	if err == nil {
		t.Fatal("expected 100x100 to exceed a 9999-pixel limit")
	}
}

func TestDecodeRejectsNonImage(t *testing.T) {
	if _, err := Decode([]byte("not an image"), generousLimits); err == nil {
		t.Fatal("expected an error for bytes that are no image")
	}
}
