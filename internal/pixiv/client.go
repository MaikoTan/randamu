package pixiv

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultAPIBase is the App API root.
	DefaultAPIBase = "https://app-api.pixiv.net"
	// DefaultAuthURL is the OAuth token endpoint.
	DefaultAuthURL = "https://oauth.secure.pixiv.net/auth/token"
	// DefaultUserAgent matches pixivpy.
	DefaultUserAgent = "PixivIOSApp/7.6.2 (iOS 12.2; iPhone9,1)"
	// DefaultReferer is the Referer required for image downloads.
	DefaultReferer = "https://app-api.pixiv.net/"

	clientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	clientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	hashSecret   = "28c1fdd170a5204386cb1313c7077b34f83e4aaf4aa829ce78c231e05b0bae2c"

	maxBodySize = 10 << 20 // JSON response body limit
)

// Options are the Client constructor parameters.
type Options struct {
	HTTPClient        *http.Client
	APIBase           string
	AuthURL           string
	MaxRetries        int
	RequestsPerSecond float64
	// TokenFile persists a refresh token rotated by the server.
	TokenFile string
	UserAgent string
	Logger    *slog.Logger
}

// Client is a minimal pixiv App API client.
type Client struct {
	http        *http.Client
	apiBase     *url.URL
	authURL     string
	maxRetries  int
	minInterval time.Duration
	tokenFile   string
	userAgent   string
	log         *slog.Logger

	mu           sync.Mutex
	accessToken  string
	refreshToken string
	userID       int64
	nextAPI      time.Time
}

// New creates a client and fills in defaults.
func New(opts Options) (*Client, error) {
	base := opts.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse API base URL: %w", err)
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.AuthURL == "" {
		opts.AuthURL = DefaultAuthURL
	}
	if opts.MaxRetries < 1 {
		opts.MaxRetries = 3
	}
	rps := opts.RequestsPerSecond
	if rps <= 0 {
		rps = 2
	}
	if opts.UserAgent == "" {
		opts.UserAgent = DefaultUserAgent
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		http:        opts.HTTPClient,
		apiBase:     baseURL,
		authURL:     opts.AuthURL,
		maxRetries:  opts.MaxRetries,
		minInterval: time.Duration(float64(time.Second) / rps),
		tokenFile:   opts.TokenFile,
		userAgent:   opts.UserAgent,
		log:         opts.Logger,
	}, nil
}

// UserID returns the logged-in user id.
func (c *Client) UserID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID
}

// Login exchanges a refresh token for an access token.
func (c *Client) Login(ctx context.Context, refreshToken string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
	sum := md5.Sum([]byte(now + hashSecret))

	form := url.Values{
		"grant_type":     {"refresh_token"},
		"refresh_token":  {refreshToken},
		"get_secure_url": {"1"},
		"client_id":      {clientID},
		"client_secret":  {clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Client-Time", now)
	req.Header.Set("X-Client-Hash", hex.EncodeToString(sum[:]))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login to pixiv: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pixiv login failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r struct {
		Response struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			User         struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("parse login response: %w", err)
	}
	if r.Response.AccessToken == "" {
		return errors.New("login response has no access_token")
	}

	c.mu.Lock()
	c.accessToken = r.Response.AccessToken
	c.userID = r.Response.User.ID
	rotated := r.Response.RefreshToken
	if rotated != "" {
		c.refreshToken = rotated
	} else {
		c.refreshToken = refreshToken
	}
	c.mu.Unlock()

	if rotated != "" && rotated != refreshToken {
		if err := c.persistRefreshToken(rotated); err != nil {
			c.log.Warn("failed to save rotated refresh_token", "error", err)
		}
	}
	return nil
}

// persistRefreshToken atomically writes a rotated token.
func (c *Client) persistRefreshToken(token string) error {
	if c.tokenFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.tokenFile), 0o700); err != nil {
		return err
	}
	tmp := c.tokenFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(token), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.tokenFile)
}

// getJSON performs an authenticated GET request and decodes JSON.
func (c *Client) getJSON(ctx context.Context, rawURL string, out any) (http.Header, error) {
	var lastErr error
	reauth := false
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return nil, err
			}
		}
		if err := c.waitAPI(ctx); err != nil {
			return nil, err
		}

		var resp *http.Response
		var body []byte
		reqErr := func() error {
			reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
			if err != nil {
				return err
			}
			c.setAPIHeaders(req)
			resp, err = c.http.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err = io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
			return err
		}()
		if reqErr != nil {
			lastErr = fmt.Errorf("request %s: %w", rawURL, reqErr)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized && !reauth:
			reauth = true
			c.mu.Lock()
			token := c.refreshToken
			c.mu.Unlock()
			if token == "" {
				return nil, errors.New("got 401 but no refresh_token is available")
			}
			if err := c.Login(ctx, token); err != nil {
				return nil, fmt.Errorf("re-login: %w", err)
			}
			continue
		case resp.StatusCode == http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("parse response %s: %w", rawURL, err)
			}
			return resp.Header, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("request %s: HTTP %d", rawURL, resp.StatusCode)
			if d, ok := retryAfter(resp.Header); ok {
				if err := sleep(ctx, d); err != nil {
					return nil, err
				}
			}
			continue
		default:
			return nil, fmt.Errorf("request %s: HTTP %d: %s", rawURL, resp.StatusCode, truncate(string(body), 200))
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request %s failed", rawURL)
	}
	return nil, lastErr
}

func (c *Client) setAPIHeaders(req *http.Request) {
	c.mu.Lock()
	token := c.accessToken
	c.mu.Unlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Referer", DefaultReferer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("App-OS", "ios")
	req.Header.Set("App-OS-Version", "12.2")
	req.Header.Set("App-Version", "7.6.2")
}

// waitAPI applies the configured request rate limit.
func (c *Client) waitAPI(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	next := c.nextAPI
	if next.Before(now) {
		next = now
	}
	c.nextAPI = next.Add(c.minInterval)
	c.mu.Unlock()

	if d := time.Until(next); d > 0 {
		return sleep(ctx, d)
	}
	return nil
}

// BookmarksPage fetches one bookmarks page; an empty cursor starts from the first page.
func (c *Client) BookmarksPage(ctx context.Context, userID int64, restrict, cursor string) (Page, error) {
	target := cursor
	if target == "" {
		q := url.Values{
			"user_id":  {strconv.FormatInt(userID, 10)},
			"restrict": {restrict},
			"filter":   {"for_ios"},
		}
		u := *c.apiBase
		u.Path = "/v1/user/bookmarks/illust"
		u.RawQuery = q.Encode()
		target = u.String()
	}
	return c.fetchPage(ctx, target)
}

// SearchPage fetches one search result page; an empty cursor starts from the first page.
func (c *Client) SearchPage(ctx context.Context, p SearchParams, cursor string) (Page, error) {
	target := cursor
	if target == "" {
		q := url.Values{
			"word":          {p.Word},
			"search_target": {p.SearchTarget},
			"sort":          {p.Sort},
			"filter":        {"for_ios"},
		}
		if p.Duration != "" {
			q.Set("duration", p.Duration)
		}
		u := *c.apiBase
		u.Path = "/v1/search/illust"
		u.RawQuery = q.Encode()
		target = u.String()
	}
	return c.fetchPage(ctx, target)
}

// RecommendedPage fetches one page of the recommendation feed.
func (c *Client) RecommendedPage(ctx context.Context, offset int, viewed []int64) (Page, error) {
	q := url.Values{
		"content_type":            {"illust"},
		"include_ranking_illusts": {"false"},
		"offset":                  {strconv.Itoa(offset)},
	}
	for _, id := range viewed {
		q.Add("viewed[]", strconv.FormatInt(id, 10))
	}
	u := *c.apiBase
	u.Path = "/v1/illust/recommended"
	u.RawQuery = q.Encode()
	return c.fetchPage(ctx, u.String())
}

func (c *Client) fetchPage(ctx context.Context, target string) (Page, error) {
	if err := c.validateCursor(target); err != nil {
		return Page{}, err
	}

	// Some endpoints may omit next_url; decoding an empty value is fine.
	var body illustList
	header, err := c.getJSON(ctx, target, &body)
	if err != nil {
		return Page{}, err
	}

	next := header.Get("X-Next-Url")
	if next == "" {
		next = body.NextURL
	}
	if next != "" {
		resolved, err := c.resolveCursor(next)
		if err != nil {
			c.log.Warn("ignoring invalid next_url", "next_url", next, "error", err)
			next = ""
		} else {
			next = resolved
		}
	}
	return Page{Illusts: body.Illusts, NextURL: next}, nil
}

// validateCursor ensures the cursor points at the configured API host.
func (c *Client) validateCursor(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL %q: %w", raw, err)
	}
	if !u.IsAbs() || u.Host != c.apiBase.Host || u.Scheme != c.apiBase.Scheme {
		return fmt.Errorf("refusing non-API host: %s", raw)
	}
	return nil
}

// resolveCursor resolves a relative next_url and validates the host.
func (c *Client) resolveCursor(raw string) (string, error) {
	ref, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	resolved := c.apiBase.ResolveReference(ref)
	if resolved.Host != c.apiBase.Host || resolved.Scheme != c.apiBase.Scheme {
		return "", fmt.Errorf("next_url host mismatch: %s", resolved)
	}
	return resolved.String(), nil
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	// Simple jitter to avoid synchronized retries.
	return d/2 + time.Duration(time.Now().UnixNano()%int64(d/2+1))
}

func retryAfter(h http.Header) (time.Duration, bool) {
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t), true
	}
	return 0, false
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
