package imagecaption

import "github.com/chipskein/cade/internal/imagefile"

// Limits for decoding an image before it goes to the model.
//
// MaxSide 1024: BenchmarkDescribeImage on a 1920x1080 terminal screenshot
// lost a line and misread file names at 512 px, read everything from
// 768 px, and took 1.7 s (RTX 3060) / 22 s (Ryzen 5 5500) at 1024 px; the
// extra 256 px leave room for real screenshots, whose text is smaller.
// It also keeps an image within vision.context_tokens (~1000 tokens).
//
// MaxPixels 64 MP: above every phone camera, and decoding stays within a
// few hundred MB; a larger header is refused before any pixel is read.
var decodeLimits = imagefile.Limits{MaxPixels: 64_000_000, MaxSide: 1024}
