// Package download implements concurrent, resumable and idempotent image downloads.
package download

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder,让本包独立完成转码
	_ "image/png"  // register PNG decoder,让本包独立完成转码
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MaikoTan/randamu/internal/metadata"
	"github.com/MaikoTan/randamu/internal/webp"
)

// Job is one image to download.
type Job struct {
	URL      string
	Filename string
	Source   string
	IllustID int64
	Page     int
	// Title、Author、Tags 是可选的来源元信息，随元数据内嵌进图片文件。
	Title  string
	Author string
	Tags   []string
}

// Status is the download result.
type Status int

const (
	// Downloaded means the file was downloaded in this run.
	Downloaded Status = iota
	// Skipped means the file already exists or the original is gone.
	Skipped
	// Rejected means the image was downloaded but failed content analysis.
	Rejected
	// Failed means retries were exhausted.
	Failed
)

// String returns a log-friendly result name.
func (s Status) String() string {
	switch s {
	case Downloaded:
		return "downloaded"
	case Skipped:
		return "skipped"
	case Rejected:
		return "rejected"
	default:
		return "failed"
	}
}

// Options are the Downloader constructor parameters.
type Options struct {
	Dir         string
	MaxRetries  int
	UserAgent   string
	Referer     string
	AllowedHost string
	// AllowedHosts 为多个允许的下载主机；非空时优先于 AllowedHost。
	AllowedHosts []string
	HTTPClient   *http.Client
	Logger      *slog.Logger
	// Analyze, when non-nil, inspects a fully downloaded image; returning
	// an error deletes the file and reports the download as Rejected.
	Analyze func(path string) error
	// ConvertWebP 为 true 时把 JPEG/PNG 转码为 WebP（GIF 与已是 WebP 的
	// 文件保持原样）。转码失败时降级保留原图。
	ConvertWebP bool
	// WebPQuality 是转码质量（1-100，0 表示使用默认值 85）。
	WebPQuality int
	// EmbedMetadata 为 true 时把下载来源信息内嵌进图片文件。
	EmbedMetadata bool
}

// Downloader downloads images into a local directory concurrently.
type Downloader struct {
	dir         string
	maxRetries  int
	userAgent   string
	referer     string
	allowedHosts []string
	http        *http.Client
	log         *slog.Logger
	analyze     func(path string) error
	convertWebP bool
	webpQuality int
	embedMeta   bool

	mu      sync.Mutex
	claimed map[string]struct{}
}

// New creates a downloader.
func New(opts Options) *Downloader {
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.MaxRetries < 1 {
		opts.MaxRetries = 3
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Downloader{
		dir:         opts.Dir,
		maxRetries:  opts.MaxRetries,
		userAgent:   opts.UserAgent,
		referer:     opts.Referer,
		allowedHosts: buildAllowedHosts(opts),
		http:        opts.HTTPClient,
		log:         opts.Logger,
		claimed:     make(map[string]struct{}),
		analyze:     opts.Analyze,
		convertWebP: opts.ConvertWebP,
		webpQuality: opts.WebPQuality,
		embedMeta:   opts.EmbedMetadata,
	}
}

// buildAllowedHosts 合并 AllowedHost 与 AllowedHosts，得到主机白名单。
// 两者都为空时返回 nil，表示不限制主机（仅限 http/https）。
func buildAllowedHosts(opts Options) []string {
	hosts := make([]string, 0, len(opts.AllowedHosts)+1)
	if opts.AllowedHost != "" {
		hosts = append(hosts, opts.AllowedHost)
	}
	hosts = append(hosts, opts.AllowedHosts...)
	if len(hosts) == 0 {
		return nil
	}
	return hosts
}

// FileName derives the file name from the original image URL as {illust_id}_p{n}.{ext}.
func FileName(illustID int64, page int, rawURL string) (string, error) {
	return FileNameFor("pixiv", illustID, page, rawURL)
}

// FileNameFor 与 FileName 相同，但非 pixiv 图源会加图源前缀，
// 避免不同图源的数字 ID 撞车（如 danbooru 与 pixiv 都从 1 开始编号）。
// 前缀在非 pixiv 图源下无条件添加，即使 URL 末尾恰好形如 {id}_p{n}.{ext}
// —— danbooru 等站点的文件名也可能长这样。
func FileNameFor(source string, illustID int64, page int, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse image URL: %w", err)
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "", fmt.Errorf("image URL has no file name: %s", rawURL)
	}
	// pixiv 已是 {id}_p{n}.{ext} 形式的直接用（兼容旧脚本下载的文件）。
	if source == "" || source == "pixiv" {
		if matched, _ := regexp.MatchString(`^[0-9]+_p[0-9]+\.[A-Za-z0-9]+$`, base); matched {
			return base, nil
		}
	}
	ext := path.Ext(base)
	if ext == "" || len(ext) > 8 {
		ext = ".jpg"
	}
	name := fmt.Sprintf("%d_p%d%s", illustID, page, ext)
	if source != "" && source != "pixiv" {
		name = source + "_" + name
	}
	return name, nil
}

// Download downloads a single job. It returns Skipped when the file already exists.
func (d *Downloader) Download(ctx context.Context, job Job) (Status, error) {
	if err := d.validateURL(job.URL); err != nil {
		return Failed, err
	}
	name := job.Filename
	if name == "" {
		var err error
		name, err = FileName(job.IllustID, job.Page, job.URL)
		if err != nil {
			return Failed, err
		}
	}
	// Accept plain file names only to prevent path traversal.
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return Failed, fmt.Errorf("invalid file name: %q", name)
	}

	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return Failed, err
	}
	// 幂等跳过：原图名与转码后的 WebP 名任一存在即跳过，
	// 避免同一张图以 jpg 与 webp 两种后缀重复占空间。
	// WebP 检查与转码开关无关：即使后续关闭转码，也不应再下载一份原图。
	webpName := strings.TrimSuffix(name, filepath.Ext(name)) + ".webp"
	if _, err := os.Stat(filepath.Join(d.dir, webpName)); err == nil {
		return Skipped, nil
	}
	final := filepath.Join(d.dir, name)
	if _, err := os.Stat(final); err == nil {
		return Skipped, nil
	}

	// Avoid downloading the same file from multiple workers in one run.
	if !d.claim(name) {
		return Skipped, nil
	}
	defer d.unclaim(name)

	part := final + ".part"
	var lastErr error
	var wait time.Duration
	for attempt := 0; attempt < d.maxRetries; attempt++ {
		if attempt > 0 {
			delay := wait
			if delay <= 0 {
				delay = backoff(attempt)
			}
			d.log.Warn("retrying download", "file", name, "attempt", attempt+1, "delay", delay, "error", lastErr)
			if err := sleep(ctx, delay); err != nil {
				return Failed, err
			}
		}

		complete, retryAfter, err := d.tryDownload(ctx, job.URL, part)
		if complete {
			if d.analyze != nil {
				if aerr := d.analyze(part); aerr != nil {
					_ = os.Remove(part)
					return Rejected, aerr
				}
			}
			// 顺序很重要：先转码，后写元信息。转码会重新生成容器，
			// 先写的元信息会随之丢失。
			finalName := name
			if d.convertWebP {
				converted, err := convertToWebP(part, d.webpQuality)
				if err != nil {
					// 转码失败降级保留原图。
					d.log.Debug("webp conversion skipped", "file", name, "error", err)
				} else if converted {
					finalName = strings.TrimSuffix(name, filepath.Ext(name)) + ".webp"
				}
			}
			if d.embedMeta {
				info := metadata.Info{
					Source:       job.Source,
					ID:           job.IllustID,
					Page:         job.Page,
					Title:        job.Title,
					Author:       job.Author,
					Tags:         job.Tags,
					URL:          job.URL,
					DownloadedAt: time.Now().UTC().Format(time.RFC3339),
				}
				if err := metadata.Write(part, info); err != nil {
					// 元信息写入是尽力而为：失败时保留无元数据的原图。
					d.log.Warn("cannot embed metadata", "file", name, "error", err)
				}
			}
			final := filepath.Join(d.dir, finalName)
			if err := os.Rename(part, final); err != nil {
				// Another process finished the same file first; treat it as success.
				if _, statErr := os.Stat(final); statErr == nil {
					_ = os.Remove(part)
					return Skipped, nil
				}
				return Failed, fmt.Errorf("rename %s: %w", final, err)
			}
			return Downloaded, nil
		}
		if err == nil {
			continue
		}
		var perm *permanentError
		if errors.As(err, &perm) {
			return Failed, perm.err
		}
		if errors.Is(err, errNotFound) {
			d.log.Info("image not found, skipping", "file", name, "url", job.URL)
			return Skipped, nil
		}
		lastErr = err
		wait = retryAfter
		if ctx.Err() != nil {
			return Failed, ctx.Err()
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("download %s failed", job.URL)
	}
	return Failed, lastErr
}

// errNotFound means the original image was deleted and is permanently skipped.
var errNotFound = errors.New("image not found")

// permanentError is an error that must not be retried.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// tryDownload performs one download attempt. complete reports whether the file is ready.
func (d *Downloader) tryDownload(ctx context.Context, rawURL, part string) (complete bool, retryAfter time.Duration, err error) {
	var offset int64
	if st, statErr := os.Stat(part); statErr == nil {
		offset = st.Size()
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, 0, &permanentError{err}
	}
	req.Header.Set("User-Agent", d.userAgent)
	if d.referer != "" {
		req.Header.Set("Referer", d.referer)
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// The server ignored Range; restart the full download.
		offset = 0
	case resp.StatusCode == http.StatusPartialContent:
		start, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != offset {
			_ = os.Remove(part)
			return false, 0, fmt.Errorf("resume offset mismatch (expected %d)", offset)
		}
		if total >= 0 && offset == total {
			return true, 0, nil
		}
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		_, total, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if ok && total == offset {
			return true, 0, nil
		}
		_ = os.Remove(part)
		return false, 0, fmt.Errorf("resume failed, restarting download")
	case resp.StatusCode == http.StatusNotFound:
		return false, 0, errNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return false, retryAfterOr(resp.Header, 30*time.Second), fmt.Errorf("HTTP 429")
	case resp.StatusCode >= 500:
		return false, retryAfterOr(resp.Header, 0), fmt.Errorf("HTTP %d", resp.StatusCode)
	default:
		return false, 0, &permanentError{fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		_ = os.Remove(part)
		return false, 0, &permanentError{fmt.Errorf("non-image response: %s", ct)}
	}

	flags := os.O_WRONLY | os.O_CREATE
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return false, 0, err
	}

	n, copyErr := io.CopyBuffer(f, resp.Body, make([]byte, 128*1024))
	closeErr := f.Close()
	if copyErr != nil {
		// Keep .part for the next resume attempt.
		return false, 0, copyErr
	}
	if closeErr != nil {
		return false, 0, closeErr
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return false, 0, fmt.Errorf("incomplete response: expected %d bytes, got %d", resp.ContentLength, n)
	}

	if err := verifyImage(part); err != nil {
		_ = os.Remove(part)
		return false, 0, &permanentError{err}
	}
	return true, 0, nil
}

// convertToWebP 把 path 指向的 JPEG/PNG 转码为 WebP 并原子替换。
// 返回 true 表示已转换；false 表示格式无需转换（GIF 动图、已是 WebP）。
// 出错时文件保持原样。
func convertToWebP(path string, quality int) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	head := make([]byte, 12)
	n, err := io.ReadFull(f, head)
	closeErr := f.Close()
	if err != nil && err != io.ErrUnexpectedEOF {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	head = head[:n]

	switch {
	// GIF 可能是动图，转码会丢动画，保持原样。
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return false, nil
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		// 已是 WebP（转码重压缩收益有限），保持原样。
		return false, nil
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		// JPEG，转码。
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		// PNG，转码。
	default:
		return false, fmt.Errorf("unrecognized image format")
	}

	// 完整重新解码后转码。
	f, err = os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return false, fmt.Errorf("decode for webp conversion: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".webp-*.part")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	// CreateTemp 默认 0600，转码后需与下载文件一致（0644）。
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return false, err
	}
	if err := webp.Encode(tmp, img, quality); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return false, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return false, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return false, err
	}
	return true, nil
}

func (d *Downloader) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse image URL: %w", err)
	}
	if len(d.allowedHosts) == 0 {
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("unsupported scheme: %s", u.Scheme)
		}
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing to download a non-https image: %s", raw)
	}
	for _, h := range d.allowedHosts {
		if u.Host == h {
			return nil
		}
	}
	return fmt.Errorf("refusing to download an image from %s: %s", u.Host, raw)
}

func (d *Downloader) claim(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.claimed[name]; ok {
		return false
	}
	d.claimed[name] = struct{}{}
	return true
}

func (d *Downloader) unclaim(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.claimed, name)
}

// parseContentRange parses "bytes start-end/total".
func parseContentRange(v string) (start, total int64, ok bool) {
	if !strings.HasPrefix(v, "bytes ") {
		return 0, -1, false
	}
	rest := strings.TrimPrefix(v, "bytes ")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return 0, -1, false
	}
	rangePart := strings.SplitN(parts[0], "-", 2)
	start = 0
	if parts[0] != "*" {
		if len(rangePart) != 2 {
			return 0, -1, false
		}
		var err error
		start, err = strconv.ParseInt(rangePart[0], 10, 64)
		if err != nil {
			return 0, -1, false
		}
	}
	total = -1
	if parts[1] != "*" {
		var err error
		total, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, -1, false
		}
	}
	return start, total, true
}

// verifyImage checks whether the file header is a supported image format.
func verifyImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 12)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return err
	}
	head = head[:n]

	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return nil // JPEG
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return nil // PNG
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return nil // GIF
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return nil // WebP
	}
	return fmt.Errorf("file content is not a recognized image")
}

func retryAfterOr(h http.Header, fallback time.Duration) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return fallback
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return fallback
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d/2 + time.Duration(time.Now().UnixNano()%int64(d/2+1))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
