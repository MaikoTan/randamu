package analyze

import (
	"image"
	"image/color"
	"testing"
)

// solid builds a uniform image.
func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// gradient builds a horizontal black-to-white ramp; soft and never sharp.
func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(x * 255 / (w - 1))
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

// checker builds a mosaic of n×n cells, each cell 32 px wide.
func checker(w, h, cell int, c1, c2 color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if ((x/cell)+(y/cell))%2 == 0 {
				img.Set(x, y, c1)
			} else {
				img.Set(x, y, c2)
			}
		}
	}
	return img
}

// qrPoster builds a synthetic QR-like poster: three white finder squares on a
// black field, each square with a dark center dot, plus a blank bottom half.
func qrPoster(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	// Finder squares of 56 px: 8px black ring, 8px white ring, 8px black dot.
	drawFinder := func(x0, y0 int) {
		for y := 0; y < 56; y++ {
			for x := 0; x < 56; x++ {
				ring := x < 8 || x >= 48 || y < 8 || y >= 48
				switch {
				case ring:
					img.Set(x0+x, y0+y, color.RGBA{0, 0, 0, 255})
				case x >= 16 && x < 40 && y >= 16 && y < 40:
					img.Set(x0+x, y0+y, color.RGBA{0, 0, 0, 255})
				}
			}
		}
	}
	drawFinder(40, 40)
	drawFinder(size-96, 40)
	drawFinder(40, size-96)
	return img
}

func TestCheckMosaic(t *testing.T) {
	// 32 px cells of strongly contrasting colors: the mosaic signature.
	img := checker(320, 320, 32, color.RGBA{20, 20, 20, 255}, color.RGBA{230, 230, 230, 255})
	res := Check(img, Options{Mosaic: true})
	if !res.Skip {
		t.Fatalf("mosaic must be rejected: %+v", res)
	}
	// A fine checkerboard is a legitimate pattern, not censorship.
	img = checker(320, 320, 4, color.RGBA{20, 20, 20, 255}, color.RGBA{230, 230, 230, 255})
	if res := Check(img, Options{Mosaic: true}); res.Skip {
		t.Fatalf("fine checkerboard must pass: %+v", res)
	}
}

func TestCheckGradientNotMosaicOrBlur(t *testing.T) {
	img := gradient(400, 300)
	if res := Check(img, Options{Mosaic: true}); res.Skip {
		t.Fatalf("smooth gradient must not be called mosaic: %+v", res)
	}
}

func TestCheckBlur(t *testing.T) {
	// A 5x5 box blur of a gradient stays smooth; nothing sharp anywhere.
	img := gradient(400, 300)
	res := Check(img, Options{Blur: true})
	if !res.Skip {
		t.Fatalf("smooth gradient must be rejected as blurred: %+v", res)
	}
	if res.SharpRatio > 0.001 {
		t.Fatalf("sharp ratio unexpectedly high: %v", res.SharpRatio)
	}
	// Real content: a synthetic "line art" image with many hard edges.
	img = checker(400, 300, 6, color.RGBA{10, 10, 10, 255}, color.RGBA{240, 240, 240, 255})
	if res := Check(img, Options{Blur: true}); res.Skip {
		t.Fatalf("detailed image must pass the blur check: %+v", res)
	}
}

func TestCheckQR(t *testing.T) {
	img := qrPoster(400)
	res := Check(img, Options{QR: true})
	if !res.Skip {
		t.Fatalf("QR poster must be rejected: %+v", res)
	}
	if res.QRCoverage < 0.08 {
		t.Fatalf("coverage too low: %v", res.QRCoverage)
	}
	// A plain image has no finder patterns.
	if res := Check(solid(400, 400, color.RGBA{128, 128, 128, 255}), Options{QR: true}); res.Skip {
		t.Fatalf("plain image must pass: %+v", res)
	}
}

// qrOnCanvas draws a small QR-like symbol (three 56 px finders with a 120 px
// span) on a light canvas, like a promo QR in the corner of an illustration.
func qrOnCanvas(w, h, x0, y0 int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	drawFinder := func(fx, fy int) {
		for y := 0; y < 56; y++ {
			for x := 0; x < 56; x++ {
				ring := x < 8 || x >= 48 || y < 8 || y >= 48
				center := x >= 16 && x < 40 && y >= 16 && y < 40
				if ring || center {
					img.Set(fx+x, fy+y, color.RGBA{0, 0, 0, 255})
				}
			}
		}
	}
	drawFinder(x0, y0)
	drawFinder(x0+120, y0)
	drawFinder(x0, y0+120)
	return img
}

func TestCheckQRSmallCoverage(t *testing.T) {
	img := qrOnCanvas(1000, 700, 100, 100)
	res := Check(img, Options{QR: true})
	if !res.Skip {
		t.Fatalf("small promo QR must be rejected: %+v", res)
	}
	if res.QRCoverage < 0.02 || res.QRCoverage > 0.08 {
		t.Fatalf("coverage = %v, want a few percent", res.QRCoverage)
	}
	// The threshold is configurable.
	if res := Check(img, Options{QR: true, QRMinCoverage: 0.10}); res.Skip {
		t.Fatalf("10%% threshold must let a 4%% QR pass: %+v", res)
	}
}

func TestCheckDisabled(t *testing.T) {
	img := checker(320, 320, 32, color.RGBA{20, 20, 20, 255}, color.RGBA{230, 230, 230, 255})
	if res := Check(img, Options{}); res.Skip {
		t.Fatalf("no detection enabled must never skip: %+v", res)
	}
}

func TestCheckSmallImage(t *testing.T) {
	if res := Check(solid(8, 8, color.RGBA{0, 0, 0, 255}), Options{QR: true, Mosaic: true, Blur: true}); res.Skip {
		t.Fatalf("tiny image must be kept: %+v", res)
	}
}
