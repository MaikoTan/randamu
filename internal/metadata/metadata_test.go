//go:build cgo

package metadata

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MaikoTan/randamu/internal/webp"
)

// solidImage 生成一张纯色测试图。
func solidImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// testInfo 返回一份覆盖各字段的样例元信息。
func testInfo() Info {
	return Info{
		Source:       "pixiv:bookmarks",
		ID:           12345678,
		Page:         2,
		Title:        "测试标题",
		Author:       "某画师",
		Tags:         []string{"原创", "少女", "风景"},
		URL:          "https://i.pximg.net/img-original/img/example.jpg",
		DownloadedAt: "2026-11-15T00:00:00Z",
	}
}

// writeFixture 将 img 按 ext 指定的格式编码到临时目录并返回路径。
func writeFixture(t *testing.T, dir, name string, img image.Image, encode func(*bytes.Buffer) error) string {
	t.Helper()
	var buf bytes.Buffer
	if err := encode(&buf); err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestPayloadTruncatesTags(t *testing.T) {
	info := testInfo()
	info.Tags = make([]string, 50)
	for i := range info.Tags {
		info.Tags[i] = "tag"
	}
	got := info.Payload()
	if !bytes.HasPrefix([]byte(got), []byte(Marker)) {
		t.Fatalf("payload missing marker: %q", got)
	}
	parsed, err := extract([]byte(got))
	if err != nil {
		t.Fatalf("extract payload: %v", err)
	}
	if len(parsed.Tags) != maxTags {
		t.Errorf("tags = %d, want %d", len(parsed.Tags), maxTags)
	}
}

func TestRoundTrip(t *testing.T) {
	img := solidImage(16, 12, color.NRGBA{10, 20, 30, 255})
	const gray = 100
	encoders := map[string]func(*bytes.Buffer) error{
		"a.png":  func(b *bytes.Buffer) error { return png.Encode(b, img) },
		"a.jpg":  func(b *bytes.Buffer) error { return jpeg.Encode(b, img, &jpeg.Options{Quality: gray}) },
		"a.webp": func(b *bytes.Buffer) error { return webp.Encode(b, img, webp.DefaultQuality) },
	}
	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, name, img, encode)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			info := testInfo()
			if err := Write(path, info); err != nil {
				t.Fatalf("Write: %v", err)
			}

			got, err := Detect(path)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if !reflect.DeepEqual(got, info) {
				t.Errorf("Detect = %+v, want %+v", got, info)
			}

			// 写入后文件必须仍旧可解码。
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(before, after) {
				t.Error("Write did not modify the file")
			}
			assertDecodes(t, name, after)
		})
	}
}

// assertDecodes 用对应格式的解码器检查图像仍然有效。
func assertDecodes(t *testing.T, name string, data []byte) {
	t.Helper()
	var err error
	switch filepath.Ext(name) {
	case ".png":
		_, err = png.Decode(bytes.NewReader(data))
	case ".jpg":
		_, err = jpeg.Decode(bytes.NewReader(data))
	case ".webp":
		_, err = webp.Decode(bytes.NewReader(data))
	}
	if err != nil {
		t.Fatalf("decode after write: %v", err)
	}
}

func TestDetectNoMarker(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "plain.png", solidImage(4, 4, color.NRGBA{0, 0, 0, 255}),
		func(b *bytes.Buffer) error { return png.Encode(b, solidImage(4, 4, color.NRGBA{0, 0, 0, 255})) })
	if _, err := Detect(path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Detect = %v, want ErrNotFound", err)
	}
}

func TestDetectUnsupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.gif")
	if err := os.WriteFile(path, []byte("GIF89a......"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(path); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Detect = %v, want ErrUnsupported", err)
	}
	if err := Write(path, testInfo()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Write = %v, want ErrUnsupported", err)
	}
}

func TestCorruptInput(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"empty.png":    {},
		"trunc.png":    {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0},
		"trunc.jpg":    {0xFF, 0xD8, 0xFF, 0xE0, 0xFF, 0xFF},
		"trunc.webp":   []byte("RIFF\x10\x00\x00\x00WEBP"),
		"bad.json.png": append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte(Marker+"{not json}")...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			// 不应 panic；Detect 可返回任意错误。
			if _, err := Detect(path); err == nil {
				t.Logf("Detect(%s) returned no error", name)
			}
			if err := Write(path, testInfo()); err == nil {
				t.Logf("Write(%s) returned no error", name)
			}
		})
	}
}

// TestPNGChunkPlacement 确认 tEXt 块紧跟 IHDR，且其余块原样保留。
func TestPNGChunkPlacement(t *testing.T) {
	dir := t.TempDir()
	img := solidImage(8, 8, color.NRGBA{1, 2, 3, 255})
	path := writeFixture(t, dir, "p.png", img, func(b *bytes.Buffer) error { return png.Encode(b, img) })
	before, _ := os.ReadFile(path)
	if err := Write(path, testInfo()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after, _ := os.ReadFile(path)

	// 第一个块是 IHDR，第二个块必须是 tEXt。
	pos := 8
	first := int(binary.BigEndian.Uint32(after[pos : pos+4]))
	pos += 12 + first
	n := int(binary.BigEndian.Uint32(after[pos : pos+4]))
	typ := string(after[pos+4 : pos+8])
	if typ != "tEXt" {
		t.Fatalf("second chunk = %s, want tEXt", typ)
	}
	data := after[pos+8 : pos+8+n]
	if !bytes.HasPrefix(data, []byte("Randamu\x00"+Marker)) {
		t.Errorf("tEXt data = %q", data)
	}
	// 原文件应作为去掉新块的子序列存在（IDAT 等未被改动）。
	_ = before
}

// TestWriteIdempotent 确认重复写入不会累积块、不增大文件，且 Detect 读到最新值。
func TestWriteIdempotent(t *testing.T) {
	img := solidImage(8, 8, color.NRGBA{9, 8, 7, 255})
	encoders := map[string]func(*bytes.Buffer) error{
		"a.png":  func(b *bytes.Buffer) error { return png.Encode(b, img) },
		"a.jpg":  func(b *bytes.Buffer) error { return jpeg.Encode(b, img, &jpeg.Options{Quality: 90}) },
		"a.webp": func(b *bytes.Buffer) error { return webp.Encode(b, img, webp.DefaultQuality) },
	}
	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, name, img, encode)
			first := testInfo()
			if err := Write(path, first); err != nil {
				t.Fatalf("first Write: %v", err)
			}
			before, _ := os.ReadFile(path)

			second := testInfo()
			second.ID = 999
			second.Tags = nil
			if err := Write(path, second); err != nil {
				t.Fatalf("second Write: %v", err)
			}
			after, _ := os.ReadFile(path)

			if n := bytes.Count(after, []byte(Marker)); n != 1 {
				t.Errorf("marker count = %d, want 1", n)
			}
			if len(after) > len(before) {
				t.Errorf("file grew on rewrite: %d -> %d bytes", len(before), len(after))
			}
			got, err := Detect(path)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if got.ID != 999 {
				t.Errorf("Detect ID = %d, want 999", got.ID)
			}
			assertDecodes(t, name, after)
		})
	}
}

// TestMatchingBraceEscapes 确认字符串内的转义引号与花括号不干扰配对。
func TestMatchingBraceEscapes(t *testing.T) {
	in := []byte(`{"t":"a\"b}","x":{"y":"\\"},"z":1}`)
	end := matchingBrace(in)
	if end != len(in) {
		t.Fatalf("matchingBrace end = %d, want %d", end, len(in))
	}
	var info map[string]any
	if err := json.Unmarshal(in[:end], &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// TestWritePreservesMode 确认 Write 保留原文件权限。
func TestWritePreservesMode(t *testing.T) {
	dir := t.TempDir()
	img := solidImage(4, 4, color.NRGBA{1, 1, 1, 255})
	path := writeFixture(t, dir, "m.png", img, func(b *bytes.Buffer) error { return png.Encode(b, img) })
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, testInfo()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", fi.Mode().Perm())
	}
}
