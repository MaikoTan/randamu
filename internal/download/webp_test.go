package download

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MaikoTan/randamu/internal/metadata"
)

// makeJPEG 生成一张可被完整解码的 JPEG 图片字节。
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// 转码开启时，JPEG 下载后应变成 {id}_p0.webp，原 jpg 不复存在。
func TestDownloadConvertsToWebP(t *testing.T) {
	body := makeJPEG(t, 64, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := New(Options{
		Dir:         t.TempDir(),
		MaxRetries:  3,
		AllowedHost: "",
		ConvertWebP: true,
		WebPQuality: 85,
	})
	name := "12345678_p0.jpg"
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Downloaded {
		t.Fatalf("download: status=%v err=%v", status, err)
	}

	webpPath := filepath.Join(d.dir, "12345678_p0.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Fatalf("converted webp missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.dir, name)); !os.IsNotExist(err) {
		t.Fatal("original jpg must be replaced by webp")
	}

	// 再次下载同一图应跳过（webp 已存在）。
	status, err = d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Skipped {
		t.Fatalf("re-download must skip when webp exists: status=%v err=%v", status, err)
	}
}

// 转码关闭时保留原图。
func TestDownloadKeepsOriginalWithoutWebP(t *testing.T) {
	body := makeJPEG(t, 32, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := New(Options{Dir: t.TempDir(), MaxRetries: 3, AllowedHost: ""})
	name := "12345678_p0.jpg"
	if _, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(d.dir, name), body)
}

// 开启元信息内嵌时，文件内应能找到 randamu 标记与作品 ID。
func TestDownloadEmbedsMetadata(t *testing.T) {
	body := makeJPEG(t, 32, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := New(Options{Dir: t.TempDir(), MaxRetries: 3, AllowedHost: "", EmbedMetadata: true})
	name := "777_p0.jpg"
	_, err := d.Download(context.Background(), Job{
		URL: srv.URL + "/" + name, Filename: name,
		Source: "lolicon", IllustID: 777, Page: 0, Title: "テスト", Author: "作者",
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := metadata.Detect(filepath.Join(d.dir, name))
	if err != nil {
		t.Fatalf("detect embedded metadata: %v", err)
	}
	if info.Source != "lolicon" || info.ID != 777 || info.Page != 0 {
		t.Fatalf("unexpected metadata: %+v", info)
	}
	if info.Title != "テスト" || info.Author != "作者" {
		t.Fatalf("metadata text not embedded: %+v", info)
	}
}
