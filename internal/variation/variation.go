// Package variation merges the pages of a pixiv work that are variations of
// one picture (差分) into a single animated PNG.
package variation

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/MaikoTan/randamu/internal/apng"
)

// ErrNotVariation reports that the pages are not variations of one picture:
// their sizes differ or their content is too dissimilar.
var ErrNotVariation = errors.New("pages are not variations of one image")

// Merge combines pages into {id}.png inside dir and removes the page files.
// It refuses with ErrNotVariation unless every page decodes, all pages share
// one size, and every page resembles the first one by at least minSimilarity
// (normalized cross-correlation of luma, 0..1).
func Merge(dir string, id int64, pages []string, delay time.Duration, minSimilarity float64) (string, error) {
	if len(pages) < 2 {
		return "", fmt.Errorf("%w: %d page(s)", ErrNotVariation, len(pages))
	}
	frames := make([]image.Image, 0, len(pages))
	for _, p := range pages {
		img, err := decodeFile(p)
		if err != nil {
			return "", err
		}
		if len(frames) > 0 {
			a, b := frames[0].Bounds(), img.Bounds()
			if a.Dx() != b.Dx() || a.Dy() != b.Dy() {
				return "", fmt.Errorf("%w: %s is %dx%d, first page is %dx%d",
					ErrNotVariation, p, b.Dx(), b.Dy(), a.Dx(), a.Dy())
			}
		}
		frames = append(frames, img)
	}
	first := lumaGrid(frames[0])
	firstCenter := lumaGridRect(frames[0], centerRect(frames[0].Bounds()))
	for i := 1; i < len(frames); i++ {
		full := correlation(first, lumaGrid(frames[i]))
		center := correlation(firstCenter, lumaGridRect(frames[i], centerRect(frames[i].Bounds())))
		// A variation may repaint the background or move the framing while the
		// subject stays put, so a near-identical central area is enough.
		if full < minSimilarity && center < centerMinSimilarity {
			return "", fmt.Errorf("%w: page %d similarity %.3f (center %.3f) below threshold",
				ErrNotVariation, i, full, center)
		}
	}

	final := filepath.Join(dir, fmt.Sprintf("%d.png", id))
	tmp, err := os.CreateTemp(dir, ".variation-*.png")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if err := apng.Encode(tmp, frames, delay); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	for _, p := range pages {
		if err := os.Remove(p); err != nil {
			return final, fmt.Errorf("remove %s: %w", p, err)
		}
	}
	return final, nil
}

// decodeFile reads one image file with the standard library decoders.
func decodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

// gridSide is the analysis resolution. Variations differ in small details;
// comparing 64x64 luma grids is enough to tell them from unrelated pages and
// cheap enough to run on full-size originals.
const gridSide = 64

// centerMinSimilarity is the correlation required for the central half of two
// pages. Variations that change the background or crop differently still keep
// the subject in place, while sequential manga pages do not.
const centerMinSimilarity = 0.97

// centerRect returns the central half of b.
func centerRect(b image.Rectangle) image.Rectangle {
	dx, dy := b.Dx()/4, b.Dy()/4
	if dx == 0 || dy == 0 {
		return b
	}
	return image.Rect(b.Min.X+dx, b.Min.Y+dy, b.Max.X-dx, b.Max.Y-dy)
}

// lumaGrid downsamples the image into a gridSide×gridSide luma grid by box
// averaging, so small shifts and compression noise do not matter.
func lumaGrid(img image.Image) []float64 {
	return lumaGridRect(img, img.Bounds())
}

// lumaGridRect is lumaGrid over one rectangle of the image.
func lumaGridRect(img image.Image, b image.Rectangle) []float64 {
	w, h := b.Dx(), b.Dy()
	grid := make([]float64, gridSide*gridSide)
	for gy := 0; gy < gridSide; gy++ {
		y0 := b.Min.Y + gy*h/gridSide
		y1 := b.Min.Y + (gy+1)*h/gridSide
		for gx := 0; gx < gridSide; gx++ {
			x0 := b.Min.X + gx*w/gridSide
			x1 := b.Min.X + (gx+1)*w/gridSide
			var sum, count float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bb, _ := img.At(x, y).RGBA()
					sum += float64((299*r+587*g+114*bb)/1000) / 256
					count++
				}
			}
			if count > 0 {
				grid[gy*gridSide+gx] = sum / count
			}
		}
	}
	return grid
}

// correlation returns the normalized cross-correlation of two grids, in
// [-1, 1]. It is insensitive to overall brightness and color shifts, so a
// variation that recolors clothing still correlates high, while unrelated
// pages do not.
func correlation(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	n := float64(len(a))
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= n
	mb /= n
	var num, da, db float64
	for i := range a {
		x := a[i] - ma
		y := b[i] - mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		// Flat pages carry no structure; only equal means count as the same.
		if da == 0 && db == 0 && math.Abs(ma-mb) < 1 {
			return 1
		}
		return 0
	}
	return num / math.Sqrt(da*db)
}
