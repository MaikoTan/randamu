package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MaikoTan/randamu/internal/config"
	"github.com/MaikoTan/randamu/internal/pixiv"
)

func TestLoliconRequestAndMapping(t *testing.T) {
	var gotBody loliconRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": []map[string]any{
				{
					"pid": 1, "uid": 10, "title": "t1", "author": "a1",
					"tags": []string{"x", "y"}, "r18": true,
					"urls": map[string]string{"original": "https://orig/1.jpg", "regular": "https://reg/1.jpg"},
				},
				{
					"pid": 2, "uid": 20, "title": "t2", "author": "a2",
					"tags": []string{"z"}, "r18": false,
					"urls": map[string]string{"regular": "https://reg/2.jpg"},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Source{Type: "lolicon", Tags: []string{"tagA"}, Limit: 5, APIBase: srv.URL}
	src := NewLolicon(cfg, Options{HTTP: srv.Client()})
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 请求体形状
	if !gotBody.ExcludeAI || gotBody.Proxy != "null" || gotBody.Num != 5 || gotBody.R18 != 0 {
		t.Errorf("request body = %+v", gotBody)
	}
	if len(gotBody.Tag) != 1 || gotBody.Tag[0] != "tagA" {
		t.Errorf("tag = %v", gotBody.Tag)
	}
	if len(gotBody.Size) != 2 || gotBody.Size[0] != "regular" || gotBody.Size[1] != "original" {
		t.Errorf("size = %v", gotBody.Size)
	}

	if page.NextCursor != "" {
		t.Errorf("NextCursor = %q", page.NextCursor)
	}
	if len(page.Works) != 2 {
		t.Fatalf("works = %+v", page.Works)
	}
	w0 := page.Works[0]
	if w0.ID != 1 || w0.AuthorID != 10 || w0.AI {
		t.Errorf("w0 = %+v", w0)
	}
	if w0.XRestrict != 1 || w0.Pages[0].URL != "https://orig/1.jpg" {
		t.Errorf("w0 original preference failed: %+v", w0)
	}
	// 无 original 时退回 regular
	if page.Works[1].Pages[0].URL != "https://reg/2.jpg" || page.Works[1].XRestrict != 0 {
		t.Errorf("w1 fallback failed: %+v", page.Works[1])
	}
}

func TestKonachanCursorRatingAndAI(t *testing.T) {
	var gotQuery string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "tags": "tag1 ai_generated", "rating": "e", "file_url": "https://k/1.jpg", "sample_url": "https://k/s1.jpg"},
			{"id": 2, "tags": "tag2", "rating": "s", "sample_url": "https://k/s2.jpg"},
		})
	}))
	defer srv.Close()

	cfg := config.Source{Type: "konachan", Tags: []string{"solo"}, Rating: "questionable", Limit: 2, APIBase: srv.URL}
	src := NewKonachan(cfg, Options{HTTP: srv.Client()})

	// 空 cursor 即第 1 页
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotQuery == "" || !strings.Contains(gotQuery, "page=1") || !strings.Contains(gotQuery, "rating%3Aq") {
		t.Errorf("query = %s", gotQuery)
	}
	if len(page.Works) != 2 {
		t.Fatalf("works = %+v", page.Works)
	}
	if !page.Works[0].AI || page.Works[0].XRestrict != 1 {
		t.Errorf("AI/R18 detection failed: %+v", page.Works[0])
	}
	if page.Works[0].Pages[0].URL != "https://k/1.jpg" {
		t.Errorf("file_url preference failed: %+v", page.Works[0])
	}
	// file_url 缺失时退回 sample_url
	if page.Works[1].Pages[0].URL != "https://k/s2.jpg" {
		t.Errorf("sample_url fallback failed: %+v", page.Works[1])
	}
	// 满页 → 下一页
	if page.NextCursor != "2" {
		t.Errorf("NextCursor = %q", page.NextCursor)
	}

	// 传输 cursor 页码
	_, err = src.Fetch(context.Background(), "5")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(gotQuery, "page=5") {
		t.Errorf("query = %s", gotQuery)
	}
}

func TestDanbooruCursorRatingAndAI(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "tag_string": "a b ai-generated", "rating": "e", "file_url": "https://d/1.jpg", "uploader_name": "u1"},
			{"id": 2, "tag_string": "c", "rating": "s", "large_file_url": "https://d/l2.jpg"},
		})
	}))
	defer srv.Close()

	cfg := config.Source{Type: "danbooru", Tags: []string{"solo"}, Rating: "safe", Limit: 2, APIBase: srv.URL}
	src := NewDanbooru(cfg, Options{HTTP: srv.Client()})
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(gotQuery, "rating%3Ag%2Cs") || !strings.Contains(gotQuery, "page=1") {
		t.Errorf("query = %s", gotQuery)
	}
	if len(page.Works) != 2 {
		t.Fatalf("works = %+v", page.Works)
	}
	if !page.Works[0].AI || page.Works[0].XRestrict != 1 || page.Works[0].Author != "u1" {
		t.Errorf("w0 = %+v", page.Works[0])
	}
	if page.Works[1].Pages[0].URL != "https://d/l2.jpg" || page.Works[1].XRestrict != 0 {
		t.Errorf("w1 = %+v", page.Works[1])
	}
	if page.NextCursor != "2" {
		t.Errorf("NextCursor = %q", page.NextCursor)
	}
}

func TestPixivMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"illusts": []map[string]any{
				{
					"id": 100, "title": "T", "page_count": 2,
					"illust_ai_type": 2, "x_restrict": 1,
					"user": map[string]any{"id": 7, "name": "author"},
					"tags": []map[string]any{{"name": "tag1"}, {"name": "tag2"}},
					"meta_pages": []map[string]any{
						{"image_urls": map[string]string{"original": "https://i/100_p0.jpg"}},
						{"image_urls": map[string]string{"original": "https://i/100_p1.jpg"}},
					},
				},
				{
					"id": 101, "title": "T2", "page_count": 1,
					"meta_single_page": map[string]any{"original_image_url": "https://i/101_p0.jpg"},
				},
			},
			"next_url": "/v1/next",
		})
	}))
	defer srv.Close()

	client, err := pixiv.New(pixiv.Options{
		HTTPClient:        srv.Client(),
		APIBase:           srv.URL,
		MaxRetries:        1,
		RequestsPerSecond: 1000,
	})
	if err != nil {
		t.Fatalf("pixiv.New: %v", err)
	}
	src := NewPixiv(client, "bookmarks", config.Source{Restrict: "public"}, 1)
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(page.Works) != 2 {
		t.Fatalf("works = %+v", page.Works)
	}
	w := page.Works[0]
	if w.ID != 100 || w.Author != "author" || w.AuthorID != 7 || !w.AI || w.XRestrict != 1 {
		t.Errorf("w = %+v", w)
	}
	if len(w.Tags) != 2 || w.Tags[0] != "tag1" {
		t.Errorf("tags = %v", w.Tags)
	}
	if len(w.Pages) != 2 || w.Pages[0].Index != 0 || w.Pages[1].URL != "https://i/100_p1.jpg" {
		t.Errorf("pages = %+v", w.Pages)
	}
	if page.Works[1].Pages[0].URL != "https://i/101_p0.jpg" {
		t.Errorf("single page = %+v", page.Works[1].Pages)
	}
	if page.NextCursor != srv.URL+"/v1/next" {
		t.Errorf("NextCursor = %q", page.NextCursor)
	}
}

// mapIllusts 必须保留翻译名，否则仅靠 translated_name 命中的黑名单会失效。
func TestPixivMappingKeepsTranslatedTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"illusts": []map[string]any{{
				"id": 200, "title": "T", "page_count": 1,
				"user": map[string]any{"id": 1, "name": "a"},
				"tags": []map[string]any{
					{"name": "風景", "translated_name": "landscape"},
					{"name": "same", "translated_name": "same"},
					{"name": "noTrans"},
				},
				"meta_single_page": map[string]any{"original_image_url": "https://i/200_p0.jpg"},
			}},
		})
	}))
	defer srv.Close()

	client, err := pixiv.New(pixiv.Options{
		HTTPClient:        srv.Client(),
		APIBase:           srv.URL,
		MaxRetries:        1,
		RequestsPerSecond: 1000,
	})
	if err != nil {
		t.Fatalf("pixiv.New: %v", err)
	}
	src := NewPixiv(client, "bookmarks", config.Source{Restrict: "public"}, 1)
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got := page.Works[0].Tags
	want := []string{"風景", "landscape", "same", "noTrans"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tags = %v, want %v", got, want)
	}
}

func TestLoliconR18Mapping(t *testing.T) {
	cases := map[string]int{
		"":             0,
		"safe":         0,
		"questionable": 0,
		"explicit":     1,
		"r18":          1,
		"bogus":        0,
	}
	for rating, want := range cases {
		if got := loliconR18(rating); got != want {
			t.Errorf("loliconR18(%q) = %d, want %d", rating, got, want)
		}
	}
}

// explicit 评级应请求 r18=1。
func TestLoliconExplicitRequestsR18(t *testing.T) {
	var gotBody loliconRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{}})
	}))
	defer srv.Close()

	cfg := config.Source{Type: "lolicon", Rating: "explicit", Limit: 3, APIBase: srv.URL}
	if _, err := NewLolicon(cfg, Options{HTTP: srv.Client()}).Fetch(context.Background(), ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotBody.R18 != 1 || gotBody.Num != 3 {
		t.Errorf("request body = %+v", gotBody)
	}
}

// 短页表示没有更多内容，NextCursor 应为空；空页也不得重置回第 1 页。
func TestKonachanShortAndEmptyPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// 短页：只返回 1 条，less than limit
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "tags": "t", "rating": "s", "file_url": "https://k/1.jpg"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	src := NewKonachan(config.Source{Type: "konachan", Limit: 5, APIBase: srv.URL}, Options{HTTP: srv.Client()})
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.NextCursor != "" {
		t.Errorf("short page NextCursor = %q, want empty", page.NextCursor)
	}
	// 空页同样不应给出 cursor
	page, err = src.Fetch(context.Background(), "2")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(page.Works) != 0 || page.NextCursor != "" {
		t.Errorf("empty page = %+v", page)
	}
}

// 随机模式只取一批，NextCursor 必须为空。
func TestKonachanRandomNoCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回满页，验证随机模式仍不给出 cursor
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "tags": "t", "rating": "s", "file_url": "https://k/1.jpg"},
			{"id": 2, "tags": "t", "rating": "s", "file_url": "https://k/2.jpg"},
		})
	}))
	defer srv.Close()

	src := NewKonachan(config.Source{Type: "konachan", Limit: 2, Random: true, APIBase: srv.URL}, Options{HTTP: srv.Client()})
	page, err := src.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.NextCursor != "" {
		t.Errorf("random NextCursor = %q, want empty", page.NextCursor)
	}
}

func TestNewUnknownType(t *testing.T) {
	if _, err := New(config.Source{Type: "nope"}, Options{}); err == nil {
		t.Fatal("unknown type must fail")
	}
	if _, err := New(config.Source{Type: "bookmarks"}, Options{}); err == nil {
		t.Fatal("pixiv source without client must fail")
	}
}
