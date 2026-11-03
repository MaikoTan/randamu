package apng

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	_ "image/png" // register the PNG decoder for the fallback-image check
	"testing"
	"time"
)

func solidImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

type chunk struct {
	typ  string
	data []byte
}

// parseChunks checks the PNG structure and CRCs and returns the chunks.
func parseChunks(t *testing.T, b []byte) []chunk {
	t.Helper()
	if len(b) < len(signature) || !bytes.Equal(b[:len(signature)], signature) {
		t.Fatal("missing PNG signature")
	}
	b = b[len(signature):]
	var out []chunk
	for len(b) > 0 {
		if len(b) < 12 {
			t.Fatalf("truncated chunk header, %d bytes left", len(b))
		}
		n := int(binary.BigEndian.Uint32(b[0:4]))
		typ := string(b[4:8])
		if len(b) < 12+n {
			t.Fatalf("chunk %s claims %d bytes, %d left", typ, n, len(b)-12)
		}
		data := b[8 : 8+n]
		want := binary.BigEndian.Uint32(b[8+n : 12+n])
		crc := crc32.NewIEEE()
		crc.Write(b[4 : 8+n])
		if got := crc.Sum32(); got != want {
			t.Fatalf("chunk %s bad CRC: got %08x want %08x", typ, got, want)
		}
		out = append(out, chunk{typ, data})
		b = b[12+n:]
	}
	return out
}

func TestEncodeStructure(t *testing.T) {
	frames := []image.Image{
		solidImage(8, 6, color.NRGBA{255, 0, 0, 255}),
		solidImage(8, 6, color.NRGBA{0, 255, 0, 128}),
		solidImage(8, 6, color.NRGBA{0, 0, 255, 255}),
	}
	var buf bytes.Buffer
	if err := Encode(&buf, frames, 500*time.Millisecond); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	chunks := parseChunks(t, buf.Bytes())

	var types []string
	for _, c := range chunks {
		types = append(types, c.typ)
	}
	want := []string{"IHDR", "acTL", "fcTL", "IDAT", "fcTL", "fdAT", "fcTL", "fdAT", "IEND"}
	if len(types) != len(want) {
		t.Fatalf("chunks = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("chunk %d = %s, want %s (all: %v)", i, types[i], want[i], types)
		}
	}

	actl := chunks[1].data
	if got := binary.BigEndian.Uint32(actl[0:4]); got != 3 {
		t.Errorf("acTL num_frames = %d, want 3", got)
	}
	if got := binary.BigEndian.Uint32(actl[4:8]); got != 0 {
		t.Errorf("acTL num_plays = %d, want 0 (forever)", got)
	}

	// Sequence numbers count every fcTL and fdAT in order.
	var seqs []uint32
	for _, c := range chunks {
		switch c.typ {
		case "fcTL":
			seqs = append(seqs, binary.BigEndian.Uint32(c.data[0:4]))
		case "fdAT":
			seqs = append(seqs, binary.BigEndian.Uint32(c.data[0:4]))
		}
	}
	for i, s := range seqs {
		if s != uint32(i) {
			t.Fatalf("sequence numbers = %v, want 0..%d", seqs, len(seqs)-1)
		}
	}

	// Every frame carries the requested delay.
	for _, c := range chunks {
		if c.typ != "fcTL" {
			continue
		}
		num := binary.BigEndian.Uint16(c.data[20:22])
		den := binary.BigEndian.Uint16(c.data[22:24])
		if num != 500 || den != 1000 {
			t.Errorf("delay = %d/%d, want 500/1000", num, den)
		}
	}

	// A plain PNG decoder must still show the first frame.
	img, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("image.Decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 6 {
		t.Fatalf("decoded bounds = %v", b)
	}
	r, g, bb, a := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || bb>>8 != 0 || a>>8 != 255 {
		t.Errorf("decoded pixel = %d,%d,%d,%d, want opaque red", r>>8, g>>8, bb>>8, a>>8)
	}
}

func TestEncodeRejectsBadFrames(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, []image.Image{solidImage(4, 4, color.NRGBA{0, 0, 0, 255})}, time.Second); err == nil {
		t.Error("single frame: want error")
	}
	frames := []image.Image{
		solidImage(4, 4, color.NRGBA{0, 0, 0, 255}),
		solidImage(5, 4, color.NRGBA{0, 0, 0, 255}),
	}
	buf.Reset()
	if err := Encode(&buf, frames, time.Second); err == nil {
		t.Error("mismatched sizes: want error")
	}
}

func TestDelayFraction(t *testing.T) {
	if num, den := delayFraction(1500 * time.Millisecond); num != 1500 || den != 1000 {
		t.Errorf("1.5s = %d/%d, want 1500/1000", num, den)
	}
	if num, den := delayFraction(0); num != 1000 || den != 1000 {
		t.Errorf("zero delay = %d/%d, want 1000/1000", num, den)
	}
	if num, den := delayFraction(120 * time.Second); num != 120 || den != 1 {
		t.Errorf("120s = %d/%d, want 120/1", num, den)
	}
}
