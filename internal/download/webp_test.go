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
	"sync/atomic"
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

// TestConversionProducesWebPWithoutExtraDecoders 验证本包自带的
// jpeg/png 解码器注册：不经其他包裹导入也能完成转码（回归 decoder 注册缺失）。
func TestConversionProducesWebPWithoutExtraDecoders(t *testing.T) {
	body := makeJPEG(t, 16, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := New(Options{Dir: t.TempDir(), MaxRetries: 3, ConvertWebP: true})
	name := "5_p0.jpg"
	if _, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name}); err != nil {
		t.Fatal(err)
	}
	webpPath := filepath.Join(d.dir, "5_p0.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Fatalf("expected webp output: %v", err)
	}
	// 转码产物应保持 0644，而非 CreateTemp 默认的 0600。
	fi, err := os.Stat(webpPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("converted file mode = %v, want 0644", fi.Mode().Perm())
	}
}

// TestSkipsExistingWebPWhenConversionOff 确认即使关闭转码，
// 已存在的 .webp 也会让下载跳过，不会重复下载原图。
func TestSkipsExistingWebPWhenConversionOff(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	d := New(Options{Dir: t.TempDir(), MaxRetries: 3, ConvertWebP: false})
	name := "9_p0.jpg"
	if err := os.WriteFile(filepath.Join(d.dir, "9_p0.webp"), []byte("RIFF....WEBP"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Skipped {
		t.Fatalf("Download = %v, %v, want Skipped", status, err)
	}
	if hits != 0 {
		t.Fatalf("existing webp must not trigger a request, got %d", hits)
	}
}

// TestValidateURLHosts 验证多主机白名单：https 必须、主机精确匹配。
func TestValidateURLHosts(t *testing.T) {
	d := New(Options{Dir: t.TempDir(), AllowedHosts: []string{"i.pximg.net", "img.example.com"}})
	cases := []struct {
		url  string
		want bool
	}{
		{"https://i.pximg.net/a.jpg", true},
		{"https://img.example.com/a.jpg", true},
		{"http://i.pximg.net/a.jpg", false},           // 非 https
		{"https://evil-i.pximg.net/a.jpg", false},     // 主机后缀不同
		{"https://i.pximg.net.evil.com/a.jpg", false}, // 主机前缀不同
		{"https://unlisted.example.com/a.jpg", false}, // 不在白名单
	}
	for _, tc := range cases {
		if err := d.validateURL(tc.url); (err == nil) != tc.want {
			t.Errorf("validateURL(%s) err = %v, want ok=%v", tc.url, err, tc.want)
		}
	}

	// 空白名单允许 http 与 https。
	open := New(Options{Dir: t.TempDir()})
	for _, u := range []string{"http://a/b.jpg", "https://a/b.jpg"} {
		if err := open.validateURL(u); err != nil {
			t.Errorf("empty allowlist must accept %s: %v", u, err)
		}
	}
	if err := open.validateURL("ftp://a/b.jpg"); err == nil {
		t.Error("empty allowlist must reject ftp")
	}
}
