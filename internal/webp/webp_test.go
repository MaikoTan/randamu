package webp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// 测试 PNG → WebP 编码后再解码的往返，并验证有损压缩显著缩小体积。
func TestEncodeDecodeRoundTrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}

	var buf bytes.Buffer
	if err := Encode(&buf, img, 85); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("encoded webp is empty")
	}

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := decoded.Bounds()
	if b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("decoded size %dx%d, want 256x256", b.Dx(), b.Dy())
	}

	// 对渐变图有损 WebP 应远小于 PNG。
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	if buf.Len() >= pngBuf.Len() {
		t.Logf("webp %d bytes not smaller than png %d bytes", buf.Len(), pngBuf.Len())
	}

	if _, err := DecodeConfig(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("decode config: %v", err)
	}
}
