package pixiv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server, tokenFile string) *Client {
	t.Helper()
	c, err := New(Options{
		HTTPClient:        srv.Client(),
		APIBase:           srv.URL,
		AuthURL:           srv.URL + "/auth",
		MaxRetries:        3,
		RequestsPerSecond: 1000,
		TokenFile:         tokenFile,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func writeAuthResponse(w http.ResponseWriter, access, refresh string, userID int64) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"response": map[string]any{
			"access_token":  access,
			"refresh_token": refresh,
			"expires_in":    3600,
			"user":          map[string]any{"id": userID},
		},
	})
}

func TestLoginAndTokenRotation(t *testing.T) {
	var gotForm map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-Client-Hash") == "" || r.Header.Get("X-Client-Time") == "" {
			t.Error("missing X-Client-* headers")
		}
		_ = r.ParseForm()
		gotForm = r.PostForm
		writeAuthResponse(w, "access-1", "rotated-token", 15229531)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "refresh_token")
	c := newTestClient(t, srv, tokenPath)
	if err := c.Login(context.Background(), "old-token"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if gotForm["grant_type"][0] != "refresh_token" || gotForm["refresh_token"][0] != "old-token" {
		t.Fatalf("unexpected form fields: %v", gotForm)
	}
	if gotForm["client_id"][0] != clientID {
		t.Fatalf("client_id = %s", gotForm["client_id"][0])
	}
	if c.UserID() != 15229531 {
		t.Fatalf("UserID = %d", c.UserID())
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("rotated token not saved: %v", err)
	}
	if string(data) != "rotated-token" {
		t.Fatalf("token file content = %q", data)
	}
}

func TestReauthOn401(t *testing.T) {
	var authHits int32
	var apiHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth" {
			n := atomic.AddInt32(&authHits, 1)
			writeAuthResponse(w, "access-"+strconv.Itoa(int(n)), "refresh", 1)
			return
		}
		if atomic.AddInt32(&apiHits, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer access-2" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"illusts": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	if err := c.Login(context.Background(), "refresh"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := c.BookmarksPage(context.Background(), 1, "public", ""); err != nil {
		t.Fatalf("BookmarksPage: %v", err)
	}
	if authHits != 2 {
		t.Fatalf("expected one re-login, got %d auth requests", authHits)
	}
	if apiHits != 2 {
		t.Fatalf("API requests = %d, want 2", apiHits)
	}
}

func TestBookmarksPageQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/bookmarks/illust" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("user_id") != "99" || q.Get("restrict") != "private" || q.Get("filter") != "for_ios" {
			t.Errorf("query = %v", q)
		}
		w.Header().Set("X-Next-Url", "/v1/user/bookmarks/illust?user_id=99&max_bookmark_id=1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"illusts": []map[string]any{{
				"id": 1, "title": "t", "page_count": 1,
				"meta_single_page": map[string]any{"original_image_url": "https://i.pximg.net/1_p0.jpg"},
			}},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	page, err := c.BookmarksPage(context.Background(), 99, "private", "")
	if err != nil {
		t.Fatalf("BookmarksPage: %v", err)
	}
	if len(page.Illusts) != 1 || page.Illusts[0].ID != 1 {
		t.Fatalf("illusts = %+v", page.Illusts)
	}
	if page.NextURL != srv.URL+"/v1/user/bookmarks/illust?user_id=99&max_bookmark_id=1" {
		t.Fatalf("next = %s", page.NextURL)
	}
}

func TestSearchPageQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("word") != "風景" || q.Get("search_target") != "exact_match_for_tags" ||
			q.Get("sort") != "popular_desc" || q.Get("duration") != "within_last_month" {
			t.Errorf("query = %v", q)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"illusts": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	_, err := c.SearchPage(context.Background(), SearchParams{
		Word:         "風景",
		SearchTarget: "exact_match_for_tags",
		Sort:         "popular_desc",
		Duration:     "within_last_month",
	}, "")
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
}

func TestRecommendedPageViewport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("content_type") != "illust" || q.Get("offset") != "30" {
			t.Errorf("query = %v", q)
		}
		if got := q["viewed[]"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
			t.Errorf("viewed = %v", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"illusts": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	if _, err := c.RecommendedPage(context.Background(), 30, []int64{1, 2}); err != nil {
		t.Fatalf("RecommendedPage: %v", err)
	}
}

func TestNextURLHostValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"illusts":  []any{},
			"next_url": "https://evil.example/v1/next",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	page, err := c.BookmarksPage(context.Background(), 1, "public", "")
	if err != nil {
		t.Fatalf("BookmarksPage: %v", err)
	}
	if page.NextURL != "" {
		t.Fatalf("next_url from a foreign host must be ignored: %s", page.NextURL)
	}
}

func TestCursorHostRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request")
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	_, err := c.BookmarksPage(context.Background(), 1, "public", "https://evil.example/v1/user/bookmarks/illust")
	if err == nil || !strings.Contains(err.Error(), "non-API host") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRefreshTokenPrecedence(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit")
	envFile := "env-token"
	credDir := filepath.Join(dir, "creds")
	stateDir := filepath.Join(dir, "state")
	for p, v := range map[string]string{
		explicit: "explicit-token\n",
		filepath.Join(credDir, "pixiv_refresh_token"): "cred-token",
		filepath.Join(stateDir, "refresh_token"):      "state-token",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	t.Setenv("CREDENTIALS_DIRECTORY", "")

	got, err := ResolveRefreshToken(explicit, stateDir)
	if err != nil || got != "explicit-token" {
		t.Fatalf("explicit file takes precedence: %q, %v", got, err)
	}

	t.Setenv("PIXIV_REFRESH_TOKEN", envFile)
	got, err = ResolveRefreshToken("", stateDir)
	if err != nil || got != envFile {
		t.Fatalf("environment variable takes precedence over credential: %q, %v", got, err)
	}

	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	t.Setenv("CREDENTIALS_DIRECTORY", credDir)
	got, err = ResolveRefreshToken("", stateDir)
	if err != nil || got != "cred-token" {
		t.Fatalf("credential: %q, %v", got, err)
	}

	t.Setenv("CREDENTIALS_DIRECTORY", "")
	got, err = ResolveRefreshToken("", stateDir)
	if err != nil || got != "state-token" {
		t.Fatalf("state directory: %q, %v", got, err)
	}
}

func TestResolveRefreshTokenMissing(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	if _, err := ResolveRefreshToken("", t.TempDir()); err == nil {
		t.Fatal("a missing token must fail")
	}
}
