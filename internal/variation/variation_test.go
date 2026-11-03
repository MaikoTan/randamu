package variation

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePNG(t *testing.T, path string, w, h int, at func(x, y int) color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, at(x, y))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func gradient(w, h int) func(x, y int) color.NRGBA {
	return func(x, y int) color.NRGBA {
		return color.NRGBA{
			R: uint8(x * 255 / max(1, w-1)),
			G: uint8(y * 255 / max(1, h-1)),
			B: uint8((x + y) * 127 / max(1, w+h-2)),
			A: 255,
		}
	}
}

func TestMergeVariations(t *testing.T) {
	dir := t.TempDir()
	p0 := filepath.Join(dir, "42_p0.png")
	p1 := filepath.Join(dir, "42_p1.png")
	base := gradient(80, 60)
	writePNG(t, p0, 80, 60, base)
	writePNG(t, p1, 80, 60, func(x, y int) color.NRGBA {
		// A small changed detail, as in a real 差分.
		if x >= 30 && x < 50 && y >= 20 && y < 40 {
			return color.NRGBA{R: 255, A: 255}
		}
		return base(x, y)
	})

	final, err := Merge(dir, 42, []string{p0, p1}, time.Second, 0.9)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if want := filepath.Join(dir, "42.png"); final != want {
		t.Errorf("final = %s, want %s", final, want)
	}
	f, err := os.Open(final)
	if err != nil {
		t.Fatalf("merged file: %v", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatalf("decode merged file: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 80 || b.Dy() != 60 {
		t.Errorf("merged bounds = %v, want 80x60", b)
	}
	for _, p := range []string{p0, p1} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("page %s still exists (err = %v)", p, err)
		}
	}
}

func TestMergeRejectsDifferentPictures(t *testing.T) {
	dir := t.TempDir()
	p0 := filepath.Join(dir, "7_p0.png")
	p1 := filepath.Join(dir, "7_p1.png")
	base := gradient(80, 60)
	writePNG(t, p0, 80, 60, base)
	// Inverted colors: same composition but anti-correlated luma. Content
	// this far apart is not a variation set.
	writePNG(t, p1, 80, 60, func(x, y int) color.NRGBA {
		c := base(x, y)
		return color.NRGBA{R: 255 - c.R, G: 255 - c.G, B: 255 - c.B, A: 255}
	})

	if _, err := Merge(dir, 7, []string{p0, p1}, time.Second, 0.9); !errors.Is(err, ErrNotVariation) {
		t.Fatalf("Merge error = %v, want ErrNotVariation", err)
	}
	for _, p := range []string{p0, p1} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("page %s must be kept: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "7.png")); !os.IsNotExist(err) {
		t.Errorf("merged file must not exist (err = %v)", err)
	}
}

func TestMergeRejectsSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	p0 := filepath.Join(dir, "9_p0.png")
	p1 := filepath.Join(dir, "9_p1.png")
	writePNG(t, p0, 80, 60, gradient(80, 60))
	writePNG(t, p1, 60, 80, gradient(60, 80))
	if _, err := Merge(dir, 9, []string{p0, p1}, time.Second, 0.9); !errors.Is(err, ErrNotVariation) {
		t.Fatalf("Merge error = %v, want ErrNotVariation", err)
	}
}

func TestMergeNeedsTwoPages(t *testing.T) {
	dir := t.TempDir()
	p0 := filepath.Join(dir, "3_p0.png")
	writePNG(t, p0, 16, 16, gradient(16, 16))
	if _, err := Merge(dir, 3, []string{p0}, time.Second, 0.9); !errors.Is(err, ErrNotVariation) {
		t.Fatalf("Merge error = %v, want ErrNotVariation", err)
	}
}

func TestCorrelationSelf(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	at := gradient(40, 40)
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, at(x, y))
		}
	}
	grid := lumaGrid(img)
	if got := correlation(grid, grid); got < 0.999999 {
		t.Errorf("self correlation = %v, want 1", got)
	}
}
