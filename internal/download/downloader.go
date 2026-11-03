// Package download implements concurrent, resumable and idempotent image downloads.
package download

import (
	"context"
	"errors"
	"fmt"
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
)

// Job is one image to download.
type Job struct {
	URL      string
	Filename string
	Source   string
	IllustID int64
	Page     int
}

// Status is the download result.
type Status int

const (
	// Downloaded means the file was downloaded in this run.
	Downloaded Status = iota
	// Skipped means the file already exists or the original is gone.
	Skipped
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
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

// Downloader downloads images into a local directory concurrently.
type Downloader struct {
	dir         string
	maxRetries  int
	userAgent   string
	referer     string
	allowedHost string
	http        *http.Client
	log         *slog.Logger

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
		allowedHost: opts.AllowedHost,
		http:        opts.HTTPClient,
		log:         opts.Logger,
		claimed:     make(map[string]struct{}),
	}
}

// FileName derives the file name from the original image URL as {illust_id}_p{n}.{ext}.
func FileName(illustID int64, page int, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse image URL: %w", err)
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "", fmt.Errorf("image URL has no file name: %s", rawURL)
	}
	if matched, _ := regexp.MatchString(`^[0-9]+_p[0-9]+\.[A-Za-z0-9]+$`, base); matched {
		return base, nil
	}
	ext := path.Ext(base)
	if ext == "" || len(ext) > 8 {
		ext = ".jpg"
	}
	return fmt.Sprintf("%d_p%d%s", illustID, page, ext), nil
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

func (d *Downloader) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse image URL: %w", err)
	}
	if d.allowedHost == "" {
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("unsupported scheme: %s", u.Scheme)
		}
		return nil
	}
	if u.Scheme != "https" || u.Host != d.allowedHost {
		return fmt.Errorf("refusing to download a non-%s image: %s", d.allowedHost, raw)
	}
	return nil
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
