package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A minimal recognizable JPEG header, enough to pass magic byte validation.
var jpegPrefix = []byte{0xFF, 0xD8, 0xFF, 0xE0}

func testDownloader(t *testing.T, maxRetries int) *Downloader {
	t.Helper()
	return New(Options{
		Dir:         t.TempDir(),
		MaxRetries:  maxRetries,
		AllowedHost: "", // allow arbitrary httptest hosts in tests
	})
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func TestFileName(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://i.pximg.net/img-original/img/2024/01/02/03/04/05/12345678_p0.jpg", "12345678_p0.jpg"},
		{"https://i.pximg.net/img-original/img/2024/01/02/03/04/05/12345678_p3.png", "12345678_p3.png"},
		{"https://i.pximg.net/weird/file.webp", "42_p2.webp"},
		{"https://i.pximg.net/", ""},
	}
	for _, tc := range cases {
		got, err := FileName(42, 2, tc.url)
		if tc.want == "" {
			if err == nil {
				t.Fatalf("%s must fail", tc.url)
			}
			continue
		}
		if err != nil {
			t.Fatalf("FileName(%s): %v", tc.url, err)
		}
		if got != tc.want {
			t.Fatalf("FileName(%s) = %s, want %s", tc.url, got, tc.want)
		}
	}
}

func TestDownloadFull(t *testing.T) {
	body := append(append([]byte{}, jpegPrefix...), []byte("payload")...)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	job := Job{URL: srv.URL + "/12345678_p0.jpg", Filename: "12345678_p0.jpg"}
	status, err := d.Download(context.Background(), job)
	if err != nil || status != Downloaded {
		t.Fatalf("Download = %v, %v", status, err)
	}
	assertFile(t, filepath.Join(d.dir, job.Filename), body)
	if _, err := os.Stat(filepath.Join(d.dir, job.Filename) + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part must be removed")
	}
}

func TestDownloadSkipsExisting(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	name := "12345678_p0.jpg"
	if err := os.WriteFile(filepath.Join(d.dir, name), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Skipped {
		t.Fatalf("Download = %v, %v", status, err)
	}
	if hits != 0 {
		t.Fatalf("existing files must not trigger a request, got %d", hits)
	}
}

func TestDownloadResumesPartial(t *testing.T) {
	body := append(append([]byte{}, jpegPrefix...), []byte("abcdef")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != fmt.Sprintf("bytes=%d-", len(jpegPrefix)) {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", len(jpegPrefix), len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[len(jpegPrefix):])
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	name := "1_p0.jpg"
	if err := os.WriteFile(filepath.Join(d.dir, name+".part"), jpegPrefix, 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Downloaded {
		t.Fatalf("Download = %v, %v", status, err)
	}
	assertFile(t, filepath.Join(d.dir, name), body)
}

func TestDownloadRestartsWhenRangeIgnored(t *testing.T) {
	body := append(append([]byte{}, jpegPrefix...), []byte("abcdef")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignore Range and return the full content.
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	name := "1_p0.jpg"
	if err := os.WriteFile(filepath.Join(d.dir, name+".part"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Downloaded {
		t.Fatalf("Download = %v, %v", status, err)
	}
	assertFile(t, filepath.Join(d.dir, name), body)
}

func TestDownloadFinalizesOn416(t *testing.T) {
	body := append(append([]byte{}, jpegPrefix...), []byte("abcdef")...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(body)))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	name := "1_p0.jpg"
	if err := os.WriteFile(filepath.Join(d.dir, name+".part"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/" + name, Filename: name})
	if err != nil || status != Downloaded {
		t.Fatalf("Download = %v, %v", status, err)
	}
	assertFile(t, filepath.Join(d.dir, name), body)
}

func TestDownloadNotFoundSkips(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	d := testDownloader(t, 3)
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err != nil || status != Skipped {
		t.Fatalf("404 must be treated as skipped: %v, %v", status, err)
	}
}

func TestDownloadForbiddenNotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err == nil || status != Failed {
		t.Fatalf("403 must fail: %v, %v", status, err)
	}
	if hits != 1 {
		t.Fatalf("403 must not be retried, got %d requests", hits)
	}
}

func TestDownloadRetriesOn429(t *testing.T) {
	body := append(append([]byte{}, jpegPrefix...), []byte("ok")...)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	defer srv.Close()

	d := testDownloader(t, 2)
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err != nil || status != Downloaded {
		t.Fatalf("must succeed after a 429 retry: %v, %v", status, err)
	}
	if hits != 2 {
		t.Fatalf("requests = %d, want 2", hits)
	}
}

func TestDownloadRejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>error</html>"))
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err == nil || status != Failed {
		t.Fatalf("HTML must fail: %v, %v", status, err)
	}
	if _, statErr := os.Stat(filepath.Join(d.dir, "x_p0.jpg.part")); !os.IsNotExist(statErr) {
		t.Fatal("a permanent failure must remove .part")
	}
}

func TestDownloadRejectsBadMagic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("this is definitely not an image"))
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	status, err := d.Download(context.Background(), Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err == nil || status != Failed {
		t.Fatalf("non-image content must fail: %v, %v", status, err)
	}
}

func TestDownloadRejectsPathTraversal(t *testing.T) {
	d := testDownloader(t, 3)
	status, err := d.Download(context.Background(), Job{
		URL:      "https://i.pximg.net/x.jpg",
		Filename: "../escape.jpg",
	})
	if err == nil || status != Failed {
		t.Fatalf("path traversal must fail: %v, %v", status, err)
	}
}

func TestDownloadKeepsPartOnCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpegPrefix)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	d := testDownloader(t, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	status, err := d.Download(ctx, Job{URL: srv.URL + "/x_p0.jpg", Filename: "x_p0.jpg"})
	if err == nil || status != Failed {
		t.Fatalf("cancellation must fail: %v, %v", status, err)
	}
	if _, statErr := os.Stat(filepath.Join(d.dir, "x_p0.jpg.part")); statErr != nil {
		t.Fatalf(".part must be kept after cancellation: %v", statErr)
	}
}
