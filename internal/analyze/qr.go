package analyze

import "math"

// finder is one detected finder-like pattern.
type finder struct {
	x, y int // center
	size int // total run length across the pattern (≈ 7 modules)
}

// qrCoverage returns the largest share of the image covered by an L-shape of
// three QR finder patterns. It only detects the finder shapes — no decoding —
// which is enough to reject QR-code posters. A finder is the
// black-white-black-white-black square in three corners of a QR code; three
// of them of equal size, sharing a row and a column at equal distances,
// delimit the code, and their span approximates its bounding box.
func qrCoverage(b *gray) float64 {
	const (
		minSpan = 12   // smallest finder worth considering, in px
		maxSpan = 1024 // QR analysis runs on an image of at most qrMaxSize px
	)
	bin := binarize(b)
	cands := finderPatterns(bin, b.w, b.h, minSpan, maxSpan)
	best := 0
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			a, c := cands[i], cands[j]
			// The two finders of one edge have the same size and share a
			// row (top edge) or a column (left edge).
			if !similarSize(a.size, c.size) {
				continue
			}
			tol := maxInt(a.size, c.size)
			horiz := abs(c.y-a.y) <= tol
			vert := abs(c.x-a.x) <= tol
			if !horiz && !vert {
				continue
			}
			span := abs(c.x - a.x)
			if v := abs(c.y - a.y); v > span {
				span = v
			}
			// A finder much larger than the span or tiny against it is
			// not a QR corner pair.
			if a.size > span || a.size*12 < span {
				continue
			}
			// A third finder of the same size completes the L-shape: it
			// sits below the horizontal pair (or right of the vertical
			// pair) at the same distance.
			third := false
			for k := 0; k < len(cands); k++ {
				if k == i || k == j {
					continue
				}
				t := cands[k]
				if !similarSize(a.size, t.size) {
					continue
				}
				leg := 0
				if horiz {
					leg = abs(t.y - a.y)
				}
				if vert {
					leg = abs(t.x - a.x)
				}
				aligned := abs(t.x-a.x) <= tol
				if vert {
					aligned = abs(t.y-a.y) <= tol
				}
				if aligned && abs(leg-span) <= tol {
					third = true
					break
				}
			}
			if !third {
				continue
			}
			// Bounding box = center-to-center span + one finder width.
			side := span + a.size
			if side*side > b.w*b.h {
				side = int(math.Sqrt(float64(b.w * b.h)))
			}
			if side > best {
				best = side
			}
		}
	}
	if best == 0 {
		return 0
	}
	coverage := float64(best*best) / float64(b.w*b.h)
	if coverage > 1 {
		coverage = 1
	}
	return coverage
}

func similarSize(a, b int) bool {
	return a*3 >= b*2 && b*3 >= a*2 // within ~1.5x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// finderPatterns scans even rows for 1:1:3:1:1 run sequences, then verifies
// each candidate against the full 7×7 finder template in both polarities.
// Returns deduplicated finder centers.
func finderPatterns(bin []uint8, w, h, minSpan, maxSpan int) []finder {
	var out []finder
	appendFinder := func(f finder) {
		for _, o := range out {
			dx := o.x - f.x
			dy := o.y - f.y
			if dx*dx+dy*dy < minSpan*minSpan {
				return
			}
		}
		out = append(out, f)
	}

	type run struct {
		val   uint8
		start int
		len   int
	}
	for y := 0; y < h; y += 2 {
		row := y * w
		// Collect runs of same-colored pixels along the row.
		var runs []run
		for x := 0; x < w; {
			val := bin[row+x]
			start := x
			for x < w && bin[row+x] == val {
				x++
			}
			runs = append(runs, run{val, start, x - start})
		}
		// Slide a 5-run window; runs alternate colors by construction.
		for i := 0; i+4 < len(runs); i++ {
			var lens [5]int
			total := 0
			for k := 0; k < 5; k++ {
				lens[k] = runs[i+k].len
				total += lens[k]
			}
			if !finderRuns(lens, minSpan, maxSpan) {
				continue
			}
			cx := runs[i].start + total/2
			cy, ok := verticalCenter(bin, w, h, cx, y, minSpan, maxSpan)
			if !ok || !verifyFinder(bin, w, h, cx, cy, total) {
				continue
			}
			appendFinder(finder{x: cx, y: cy, size: total})
		}
	}
	return out
}

// verticalCenter returns the center row of the vertically matching finder
// nearest to hint in column x, or false when the column does not contain the
// 1:1:3:1:1 pattern.
func verticalCenter(bin []uint8, w, h, x, hint, minSpan, maxSpan int) (int, bool) {
	var starts, lens []int
	for y := 0; y < h; {
		start := y
		val := bin[y*w+x]
		for y < h && bin[y*w+x] == val {
			y++
		}
		starts = append(starts, start)
		lens = append(lens, y-start)
	}
	best, found := 0, false
	for i := 0; i+4 < len(lens); i++ {
		var r [5]int
		total := 0
		for k := 0; k < 5; k++ {
			r[k] = lens[i+k]
			total += r[k]
		}
		if !finderRuns(r, minSpan, maxSpan) {
			continue
		}
		center := starts[i] + total/2
		if !found || abs(center-hint) < abs(best-hint) {
			best, found = center, true
		}
	}
	return best, found
}

// verifyFinder checks the full 7×7 module template of a finder pattern
// around (cx, cy). A one-pixel offset in each direction is tolerated for
// downsampling error, and inverted codes are detected too.
func verifyFinder(bin []uint8, w, h, cx, cy, total int) bool {
	m := (total + 3) / 7 // module width in px
	if m < 2 {
		return false
	}
	size := 7 * m
	left := cx - size/2
	top := cy - size/2
	for polarity := 0; polarity < 2; polarity++ {
		for oy := -1; oy <= 1; oy++ {
			for ox := -1; ox <= 1; ox++ {
				matches := 0
				ok := true
				for j := 0; j < 7 && ok; j++ {
					for i := 0; i < 7; i++ {
						px := left + i*m + m/2 + ox
						py := top + j*m + m/2 + oy
						if px < 0 || py < 0 || px >= w || py >= h {
							ok = false
							break
						}
						wantDark := finderModule(i, j) == 1
						if polarity == 1 {
							wantDark = !wantDark
						}
						if (bin[py*w+px] == 0) == wantDark {
							matches++
						}
					}
				}
				// 47 of 49 modules is enough to survive compression blur
				// while rejecting ordinary artwork.
				if ok && matches >= 47 {
					return true
				}
			}
		}
	}
	return false
}

// finderModule returns 1 for the dark modules of a finder pattern and 0 for
// the light ones.
func finderModule(i, j int) int {
	if i == 0 || i == 6 || j == 0 || j == 6 {
		return 1 // outer ring
	}
	if i >= 2 && i <= 4 && j >= 2 && j <= 4 {
		return 1 // center block
	}
	return 0
}

// finderRuns reports whether five run lengths match the 1:1:3:1:1 finder
// proportions, each within one module of its nominal length.
func finderRuns(r [5]int, minSpan, maxSpan int) bool {
	total := 0
	for _, v := range r {
		total += v
	}
	if total < minSpan || total > maxSpan {
		return false
	}
	m2 := 2 * total / 7 // module width, doubled to stay in integers
	if m2 <= 0 {
		return false
	}
	nominal := [5]int{1, 1, 3, 1, 1}
	for i, want := range nominal {
		d := 2*r[i] - want*m2
		if d < 0 {
			d = -d
		}
		if d > m2 {
			return false
		}
	}
	return true
}

// binarize thresholds the image into 0/1 using the midpoint of its range,
// which works for both light-on-dark and dark-on-light posters.
func binarize(b *gray) []uint8 {
	lo, hi := uint8(255), uint8(0)
	for _, v := range b.p {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	mid := uint16(lo)/2 + uint16(hi)/2
	out := make([]uint8, len(b.p))
	for i, v := range b.p {
		if uint16(v) > mid {
			out[i] = 1
		}
	}
	return out
}
