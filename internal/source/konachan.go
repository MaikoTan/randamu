package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/MaikoTan/randamu/internal/config"
)

// defaultKonachanAPIBase 是 konachan 的默认地址。
const defaultKonachanAPIBase = "https://konachan.net"

// konachanRandomPageMax 是随机翻页的最大页码。
const konachanRandomPageMax = 1000

// KonachanSource 是 konachan（Moebooru）图源适配器。
type KonachanSource struct {
	cfg     config.Source
	http    *http.Client
	apiBase string
}

// NewKonachan 构造 konachan 图源适配器。
func NewKonachan(cfg config.Source, o Options) *KonachanSource {
	base := cfg.APIBase
	if base == "" {
		base = defaultKonachanAPIBase
	}
	return &KonachanSource{cfg: cfg, http: o.HTTP, apiBase: strings.TrimRight(base, "/")}
}

// konachanPost 是 konachan post.json 的单条记录。
type konachanPost struct {
	ID        int64  `json:"id"`
	Tags      string `json:"tags"`
	Rating    string `json:"rating"`
	FileURL   string `json:"file_url"`
	SampleURL string `json:"sample_url"`
	Author    string `json:"author"`
}

// moebooruRatingPrefix 把配置的 rating 映射为 Moebooru 的 rating: 前缀。
func moebooruRatingPrefix(rating string) string {
	switch rating {
	case "questionable":
		return "rating:q"
	case "explicit":
		return "rating:e"
	default:
		return "rating:s"
	}
}

// Fetch 抓取一页作品。cursor 为十进制页码，空串表示第 1 页。
// cfg.Random 为 true 时随机取 1..1000 页并返回 NextCursor 为空。
func (s *KonachanSource) Fetch(ctx context.Context, cursor string) (Page, error) {
	limit := s.cfg.Limit
	if limit <= 0 {
		limit = 20
	}

	var page int
	random := s.cfg.Random
	if random {
		page = rand.IntN(konachanRandomPageMax) + 1
	} else if cursor == "" {
		page = 1
	} else {
		p, err := strconv.Atoi(cursor)
		if err != nil {
			return Page{}, fmt.Errorf("parse konachan page %q: %w", cursor, err)
		}
		page = p
	}

	tagList := make([]string, 0, len(s.cfg.Tags)+1)
	tagList = append(tagList, moebooruRatingPrefix(s.cfg.Rating))
	tagList = append(tagList, s.cfg.Tags...)

	q := url.Values{
		"limit": {strconv.Itoa(limit)},
		"page":  {strconv.Itoa(page)},
		"tags":  {strings.Join(tagList, " ")},
	}
	rawURL := s.apiBase + "/post.json?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, fmt.Errorf("konachan request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("konachan request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return Page{}, fmt.Errorf("konachan read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("konachan request: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var posts []konachanPost
	if err := json.Unmarshal(body, &posts); err != nil {
		return Page{}, fmt.Errorf("konachan parse response: %w", err)
	}

	works := make([]Work, 0, len(posts))
	for _, p := range posts {
		fileURL := p.FileURL
		if fileURL == "" {
			fileURL = p.SampleURL
		}
		if fileURL == "" {
			continue
		}
		tags := strings.Fields(p.Tags)
		works = append(works, Work{
			ID:        p.ID,
			Source:    "konachan",
			Author:    p.Author,
			Tags:      tags,
			PageCount: 1,
			AI:        hasAITag(tags),
			XRestrict: moebooruXRestrict(p.Rating),
			Pages:     []ImageURL{{Index: 0, URL: fileURL}},
		})
	}

	next := ""
	if !random && len(posts) == limit {
		next = strconv.Itoa(page + 1)
	}
	return Page{Works: works, NextCursor: next}, nil
}

// moebooruXRestrict 把 Moebooru rating 映射为 x_restrict：e 视为 R18。
func moebooruXRestrict(rating string) int {
	if rating == "e" {
		return 1
	}
	return 0
}

// hasAITag 检测标签中是否含 AI 生成标记。
func hasAITag(tags []string) bool {
	for _, t := range tags {
		if t == "ai_generated" || t == "ai-generated" {
			return true
		}
	}
	return false
}
