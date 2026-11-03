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
	"sort"
	"time"

	"github.com/MaikoTan/randamu/internal/apng"
)

// ErrNotVariation reports that the pages are not variations of one picture:
// their sizes differ or their content is too dissimilar.
var ErrNotVariation = errors.New("pages are not variations of one image")

// Options configures Merge.
type Options struct {
	// MinSimilarity is the required whole-frame luma correlation (0..1).
	MinSimilarity float64
	// MatchCharacter also accepts a page when the main character keeps the
	// same clothing and hair colors in the same place.
	MatchCharacter bool
}

// Merge combines pages into {id}.png inside dir and removes the page files.
// It refuses with ErrNotVariation unless every page decodes, all pages share
// one size, and every page resembles the first one: by the whole-frame
// correlation, by a near-identical central area, or, when enabled, by an
// unchanged character.
func Merge(dir string, id int64, pages []string, delay time.Duration, opts Options) (string, error) {
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
	var firstCharacter characterSignature
	characterOK := false
	if opts.MatchCharacter {
		firstCharacter, characterOK = newCharacterSignature(frames[0])
	}
	for i := 1; i < len(frames); i++ {
		full := correlation(first, lumaGrid(frames[i]))
		center := correlation(firstCenter, lumaGridRect(frames[i], centerRect(frames[i].Bounds())))
		// A variation may repaint the background or move the framing while the
		// subject stays put, so a near-identical central area is enough.
		if full >= opts.MinSimilarity || center >= centerMinSimilarity {
			continue
		}
		if characterOK {
			if current, ok := newCharacterSignature(frames[i]); ok {
				colorDist, pose, ok := firstCharacter.match(current)
				if ok && colorDist <= characterMaxColorDistance && pose >= characterMinPoseSimilarity {
					continue
				}
				return "", fmt.Errorf("%w: page %d similarity %.3f (center %.3f, character color %.1f, pose %.3f) below threshold",
					ErrNotVariation, i, full, center, colorDist, pose)
			}
		}
		return "", fmt.Errorf("%w: page %d similarity %.3f (center %.3f) below threshold",
			ErrNotVariation, i, full, center)
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

const (
	// centerMinSimilarity is the correlation required for the central half of
	// two pages. Variations that change the background or crop differently
	// still keep the subject in place, while sequential manga pages do not.
	centerMinSimilarity = 0.97
	// Character matching looks at the saturated colors of the subject (hair
	// and clothing) instead of its face, which is often occluded.
	characterSide          = 128 // downsampled analysis side
	characterCells         = 16  // spatial density grid per color
	characterLevels        = 4   // quantization levels per RGB channel
	characterTopColors     = 6   // dominant colors kept per page
	characterScoreColors   = 3   // heaviest colors used for the score
	characterMinWeight     = 0.04
	characterMinSaturation = 40.0 / 255.0
	characterMinCoverage   = 0.03
	// characterMaxColorDistance is the largest RGB distance between matched
	// colors. A different character has different hair or clothing colors.
	characterMaxColorDistance = 12.0
	// characterMinPoseSimilarity is the required correlation between the
	// spatial distributions of the matched colors. An unchanged pose keeps
	// them in place; sequential manga pages move or redraw the character.
	characterMinPoseSimilarity = 0.8
)

// rgbSample is one averaged color sample of the downsampled image.
type rgbSample struct {
	r, g, b float64
}

// saturation returns the HSV saturation of the sample, in 0..1.
func (s rgbSample) saturation() float64 {
	maxc := math.Max(s.r, math.Max(s.g, s.b))
	minc := math.Min(s.r, math.Min(s.g, s.b))
	if maxc <= 0 {
		return 0
	}
	return (maxc - minc) / maxc
}

// bin returns the quantized color index of the sample.
func (s rgbSample) bin() int {
	level := func(v float64) int {
		l := int(v) / (256 / characterLevels)
		if l >= characterLevels {
			l = characterLevels - 1
		}
		if l < 0 {
			l = 0
		}
		return l
	}
	return (level(s.r)*characterLevels+level(s.g))*characterLevels + level(s.b)
}

// characterColor is one dominant clothing or hair color and its spatial
// distribution.
type characterColor struct {
	rgb     [3]float64
	weight  float64
	density []float64
}

// characterSignature describes the saturated colors of the central subject.
type characterSignature struct {
	colors []characterColor
}

// newCharacterSignature builds the character signature of img. It reports
// false when the image has too little saturated color to compare, as with
// black-and-white manga.
func newCharacterSignature(img image.Image) (characterSignature, bool) {
	b := characterRect(img.Bounds())
	if b.Dx() < 2 || b.Dy() < 2 {
		return characterSignature{}, false
	}
	samples := rgbGridRect(img, b, characterSide)
	bins := make([]int, len(samples))
	var counts [characterLevels * characterLevels * characterLevels]int
	var sums [characterLevels * characterLevels * characterLevels][3]float64
	saturated := 0
	for i, s := range samples {
		bins[i] = -1
		if s.saturation() < characterMinSaturation {
			continue
		}
		bin := s.bin()
		bins[i] = bin
		counts[bin]++
		sums[bin][0] += s.r
		sums[bin][1] += s.g
		sums[bin][2] += s.b
		saturated++
	}
	if float64(saturated) < characterMinCoverage*float64(len(samples)) {
		return characterSignature{}, false
	}

	order := make([]int, 0, len(counts))
	for bin, count := range counts {
		if count > 0 {
			order = append(order, bin)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		return order[i] < order[j]
	})
	if len(order) > characterTopColors {
		order = order[:characterTopColors]
	}
	sig := characterSignature{colors: make([]characterColor, 0, len(order))}
	slots := make(map[int]int, len(order))
	for _, bin := range order {
		weight := float64(counts[bin]) / float64(saturated)
		if weight < characterMinWeight {
			break
		}
		slots[bin] = len(sig.colors)
		sig.colors = append(sig.colors, characterColor{
			rgb: [3]float64{
				sums[bin][0] / float64(counts[bin]),
				sums[bin][1] / float64(counts[bin]),
				sums[bin][2] / float64(counts[bin]),
			},
			weight:  weight,
			density: make([]float64, characterCells*characterCells),
		})
	}
	if len(sig.colors) == 0 {
		return characterSignature{}, false
	}
	for i, bin := range bins {
		if bin < 0 {
			continue
		}
		slot, ok := slots[bin]
		if !ok {
			continue
		}
		x, y := i%characterSide, i/characterSide
		gx := x * characterCells / characterSide
		gy := y * characterCells / characterSide
		sig.colors[slot].density[gy*characterCells+gx]++
	}
	return sig, true
}

// match compares the signature with another page. colorDist is the weighted
// RGB distance between the matched colors and pose the weighted correlation
// of their spatial distributions.
func (s characterSignature) match(other characterSignature) (colorDist, pose float64, ok bool) {
	if len(s.colors) == 0 || len(other.colors) == 0 {
		return 0, 0, false
	}
	n := len(s.colors)
	if n > characterScoreColors {
		n = characterScoreColors
	}
	var weights, dists, poses float64
	for _, c := range s.colors[:n] {
		best := -1
		bestDist := math.Inf(1)
		for j, o := range other.colors {
			if d := colorDistance(c.rgb, o.rgb); d < bestDist {
				best, bestDist = j, d
			}
		}
		if best < 0 {
			continue
		}
		weights += c.weight
		dists += c.weight * bestDist
		poses += c.weight * correlation(c.density, other.colors[best].density)
	}
	if weights == 0 {
		return 0, 0, false
	}
	return dists / weights, poses / weights, true
}

// colorDistance is the Euclidean distance between two RGB colors.
func colorDistance(a, b [3]float64) float64 {
	dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// characterRect returns the central 80% of b, where the subject usually is.
func characterRect(b image.Rectangle) image.Rectangle {
	dx, dy := b.Dx()/10, b.Dy()/10
	if dx == 0 || dy == 0 {
		return b
	}
	return image.Rect(b.Min.X+dx, b.Min.Y+dy, b.Max.X-dx, b.Max.Y-dy)
}

// rgbGridRect downsamples one rectangle into side×side averaged RGB samples.
func rgbGridRect(img image.Image, b image.Rectangle, side int) []rgbSample {
	w, h := b.Dx(), b.Dy()
	out := make([]rgbSample, side*side)
	for gy := 0; gy < side; gy++ {
		y0 := b.Min.Y + gy*h/side
		y1 := b.Min.Y + (gy+1)*h/side
		for gx := 0; gx < side; gx++ {
			x0 := b.Min.X + gx*w/side
			x1 := b.Min.X + (gx+1)*w/side
			var sr, sg, sb, count float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bb, _ := img.At(x, y).RGBA()
					sr += float64(r >> 8)
					sg += float64(g >> 8)
					sb += float64(bb >> 8)
					count++
				}
			}
			if count > 0 {
				out[gy*side+gx] = rgbSample{r: sr / count, g: sg / count, b: sb / count}
			}
		}
	}
	return out
}

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
