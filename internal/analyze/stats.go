package analyze

import "math"

const (
	// blockSide is the analysis tile size; pixiv mosaic censorship uses
	// cells of at least this size, and blur is judged tile by tile.
	blockSide = 16
	// flatSigma is the gray standard deviation below which a block is
	// "flat" — a mosaic cell or plain background.
	flatSigma = 8.0
	// abruptDiff is the mean-to-mean jump between adjacent flat blocks
	// above which the pair counts as a mosaic cell boundary.
	abruptDiff = 20.0
)

// mosaicRatios returns (flat, abrupt): the share of near-constant blocks and
// the share of adjacent flat-block pairs whose means differ sharply.
// Mosaic censorship has a very high value of both; a plain sky or a smooth
// gradient is flat but not abrupt.
func mosaicRatios(b *gray) (flat, abrupt float64) {
	cols := b.w / blockSide
	rows := b.h / blockSide
	if cols < 2 || rows < 2 {
		return 0, 0
	}
	means := make([]float64, cols*rows)
	isFlat := make([]bool, cols*rows)
	var flatCount int
	for by := 0; by < rows; by++ {
		for bx := 0; bx < cols; bx++ {
			x0, y0 := bx*blockSide, by*blockSide
			mean, dev := meanStdDev(b, x0, y0, x0+blockSide, y0+blockSide)
			i := by*cols + bx
			means[i] = mean
			isFlat[i] = dev < flatSigma
			if isFlat[i] {
				flatCount++
			}
		}
	}
	flat = float64(flatCount) / float64(cols*rows)

	var pairs, abruptPairs int
	for by := 0; by < rows; by++ {
		for bx := 0; bx < cols; bx++ {
			i := by*cols + bx
			if !isFlat[i] {
				continue
			}
			// Right and bottom neighbor only; each pair is counted once.
			if bx+1 < cols && isFlat[i+1] {
				pairs++
				if math.Abs(means[i]-means[i+1]) >= abruptDiff {
					abruptPairs++
				}
			}
			if by+1 < rows && isFlat[i+cols] {
				pairs++
				if math.Abs(means[i]-means[i+cols]) >= abruptDiff {
					abruptPairs++
				}
			}
		}
	}
	if pairs == 0 {
		return flat, 0
	}
	return flat, float64(abruptPairs) / float64(pairs)
}

// meanStdDev returns the mean and population standard deviation of a rectangle.
func meanStdDev(b *gray, x0, y0, x1, y1 int) (mean, dev float64) {
	var sum, sumSq float64
	var n float64
	for y := y0; y < y1; y++ {
		row := y * b.w
		for x := x0; x < x1; x++ {
			v := float64(b.p[row+x])
			sum += v
			sumSq += v * v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	mean = sum / n
	varc := sumSq/n - mean*mean
	if varc < 0 {
		return mean, 0
	}
	return mean, math.Sqrt(varc)
}

// sharpRatio returns the share of pixels whose Laplacian response exceeds
// lapSharp. A blurred image has almost none: a large-radius blur turns edges
// into gentle ramps, whose second derivative stays near zero. A one-pixel
// border is excluded because Sobel treats out-of-bounds as black.
func sharpRatio(b *gray) float64 {
	var sharp, total int
	for y := 1; y < b.h-1; y++ {
		for x := 1; x < b.w-1; x++ {
			i := y*b.w + x
			lap := 4*float64(b.p[i]) -
				float64(b.p[i-b.w]) - float64(b.p[i+b.w]) -
				float64(b.p[i-1]) - float64(b.p[i+1])
			total++
			if math.Abs(lap) >= lapSharp {
				sharp++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(sharp) / float64(total)
}
