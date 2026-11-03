// Package analyze inspects downloaded images and flags non-illustration
// content such as QR-code posters, mosaic-censored works and blurred teasers
// that only exist to push viewers to a paid site.
package analyze

import (
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"math"
	"os"
)

// Thresholds. They are deliberately conservative: an ordinary illustration
// must always pass, while the cheap tricks above are caught.
const (
	// qrMinCoverage is the minimum share of the image a QR-like symbol
	// must cover before the image is rejected ("a large QR code"). Promo
	// pages often put a small QR in a corner, so the bar is low.
	qrMinCoverage = 0.02
	// qrMaxSize is the longest side used for QR analysis. Finder patterns
	// need more pixels than the other checks to be recognised reliably.
	qrMaxSize = 1024
	// mosaicMinFlat is the minimum share of near-constant blocks required
	// before the image is even considered mosaic-censored.
	mosaicMinFlat = 0.5
	// mosaicMinAbrupt is the minimum share of adjacent flat-block pairs
	// whose mean differs sharply — the checkerboard signature of a mosaic.
	// Smooth gradients and plain backgrounds never produce such pairs.
	mosaicMinAbrupt = 0.2
	// minSharpRatio is the minimum share of pixels with a strong Laplacian
	// response; a fully blurred image has almost none (measured: normal
	// illustrations ~0.21, a fully blurred teaser <0.0001).
	minSharpRatio = 0.002
	// lapSharp is the |Laplacian| above which a pixel counts as a sharp
	// edge. A large-radius blur turns edges into gentle ramps, which keep a
	// moderate first derivative (Sobel) but a near-zero second derivative.
	lapSharp = 30.0
)

// Options selects which detections run.
type Options struct {
	QR     bool
	Mosaic bool
	Blur   bool
	// QRMinCoverage overrides qrMinCoverage when positive.
	QRMinCoverage float64
}

// AnyEnabled reports whether at least one detection is enabled.
func (o Options) AnyEnabled() bool { return o.QR || o.Mosaic || o.Blur }

// Result reports what the analyzer found and why.
type Result struct {
	Skip        bool
	Reason      string
	QRCoverage  float64
	MosaicRatio float64
	SharpRatio  float64
	DecodeErr   error // analysis is best-effort; a decode failure keeps the file
}

// CheckFile decodes an image file and applies every enabled detection.
// WebP cannot be decoded with the standard library; such files are kept.
func CheckFile(path string, o Options) Result {
	if !o.AnyEnabled() {
		return Result{}
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{DecodeErr: err}
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return Result{DecodeErr: fmt.Errorf("decode image: %w", err)}
	}
	return Check(img, o)
}

// Check applies every enabled detection to a decoded image.
func Check(img image.Image, o Options) Result {
	if !o.AnyEnabled() {
		return Result{}
	}
	b := toGray(img)
	// Too small to judge (icons, strips); keep the file.
	if b.w < 16 || b.h < 16 {
		return Result{}
	}

	res := Result{}
	if o.QR {
		// QR analysis uses a higher resolution than the other checks: a
		// promo QR in a corner is small and its finder patterns would be
		// lost at 512 px.
		qb := toGrayLimit(img, qrMaxSize)
		res.QRCoverage = qrCoverage(&qb)
		minCoverage := qrMinCoverage
		if o.QRMinCoverage > 0 {
			minCoverage = o.QRMinCoverage
		}
		if res.QRCoverage >= minCoverage {
			res.Skip = true
			res.Reason = fmt.Sprintf("QR code covers %.0f%% of the image", res.QRCoverage*100)
			return res
		}
	}
	if o.Mosaic {
		flat, abrupt := mosaicRatios(&b)
		res.MosaicRatio = abrupt
		if flat >= mosaicMinFlat && abrupt >= mosaicMinAbrupt {
			res.Skip = true
			res.Reason = fmt.Sprintf("mosaic covers %.0f%% of blocks", flat*100)
			return res
		}
	}
	if o.Blur {
		res.SharpRatio = sharpRatio(&b)
		if res.SharpRatio < minSharpRatio {
			res.Skip = true
			res.Reason = fmt.Sprintf("image is blurred (%.2f%% sharp pixels)", res.SharpRatio*100)
		}
	}
	return res
}

// toGray downsamples the image to at most 512 px on the longest side and
// converts it to 8-bit gray. Analysis does not need full resolution, and the
// cap keeps cost bounded for large wallpapers.
func toGray(src image.Image) gray {
	return toGrayLimit(src, 512)
}

// toGrayLimit is toGray with a configurable longest side.
func toGrayLimit(src image.Image, maxSize int) gray {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	scale := 1.0
	if w > maxSize || h > maxSize {
		if w >= h {
			scale = float64(maxSize) / float64(w)
		} else {
			scale = float64(maxSize) / float64(h)
		}
	}
	ow := int(math.Round(float64(w) * scale))
	oh := int(math.Round(float64(h) * scale))
	if ow < 1 {
		ow = 1
	}
	if oh < 1 {
		oh = 1
	}

	g := gray{w: ow, h: oh, p: make([]uint8, ow*oh)}
	for y := 0; y < oh; y++ {
		sy := int((float64(y)+0.5)/scale) + sb.Min.Y
		if sy >= sb.Max.Y {
			sy = sb.Max.Y - 1
		}
		for x := 0; x < ow; x++ {
			sx := int((float64(x)+0.5)/scale) + sb.Min.X
			if sx >= sb.Max.X {
				sx = sb.Max.X - 1
			}
			r, gg, bb, _ := src.At(sx, sy).RGBA()
			// Integer Rec.601 luma on 16-bit values, then narrow to 8-bit.
			l := (299*r + 587*gg + 114*bb) / 1000
			g.p[y*ow+x] = uint8(l >> 8)
		}
	}
	return g
}

// gray is a downsampled 8-bit grayscale image.
type gray struct {
	w, h int
	p    []uint8
}

// stdDev returns the population standard deviation of a rectangle.
func (g *gray) stdDev(x0, y0, x1, y1 int) float64 {
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	var sum, sumSq float64
	var n float64
	for y := y0; y < y1; y++ {
		row := y * g.w
		for x := x0; x < x1; x++ {
			v := float64(g.p[row+x])
			sum += v
			sumSq += v * v
			n++
		}
	}
	mean := sum / n
	varc := sumSq/n - mean*mean
	if varc < 0 {
		return 0
	}
	return math.Sqrt(varc)
}

func sobelX(b *gray, x, y int) float64 {
	at := func(dx, dy int) float64 {
		sx, sy := x+dx, y+dy
		if sx < 0 || sy < 0 || sx >= b.w || sy >= b.h {
			return 0
		}
		return float64(b.p[sy*b.w+sx])
	}
	return (at(1, -1) + 2*at(1, 0) + at(1, 1)) -
		(at(-1, -1) + 2*at(-1, 0) + at(-1, 1))
}

func sobelY(b *gray, x, y int) float64 {
	at := func(dx, dy int) float64 {
		sx, sy := x+dx, y+dy
		if sx < 0 || sy < 0 || sx >= b.w || sy >= b.h {
			return 0
		}
		return float64(b.p[sy*b.w+sx])
	}
	return (at(-1, 1) + 2*at(0, 1) + at(1, 1)) -
		(at(-1, -1) + 2*at(0, -1) + at(1, -1))
}
