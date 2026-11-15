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

// defaultDanbooruAPIBase 是 danbooru 的默认地址。
const defaultDanbooruAPIBase = "https://danbooru.donmai.us"

// danbooruRandomPageMax 是随机翻页的最大页码。
const danbooruRandomPageMax = 1000

// DanbooruSource 是 danbooru 图源适配器。
type DanbooruSource struct {
	cfg     config.Source
	http    *http.Client
	apiBase string
}

// NewDanbooru 构造 danbooru 图源适配器。
func NewDanbooru(cfg config.Source, o Options) *DanbooruSource {
	base := cfg.APIBase
	if base == "" {
		base = defaultDanbooruAPIBase
	}
	return &DanbooruSource{cfg: cfg, http: o.HTTP, apiBase: strings.TrimRight(base, "/")}
}

// danbooruPost 是 danbooru posts.json 的单条记录。
type danbooruPost struct {
	ID           int64  `json:"id"`
	TagString    string `json:"tag_string"`
	Rating       string `json:"rating"`
	FileURL      string `json:"file_url"`
	LargeFileURL string `json:"large_file_url"`
	UploaderName string `json:"uploader_name"`
}

// danbooruRatingPrefix 把配置的 rating 映射为 danbooru 的 rating: 前缀。
func danbooruRatingPrefix(rating string) string {
	switch rating {
	case "safe":
		return "rating:g,s"
	case "questionable":
		return "rating:q"
	case "explicit":
		return "rating:e"
	default:
		return ""
	}
}

// danbooruXRestrict 把 danbooru rating 映射为 x_restrict：e 视为 R18。
func danbooruXRestrict(rating string) int {
	if rating == "e" {
		return 1
	}
	return 0
}

// Fetch 抓取一页作品。cursor 为十进制页码，空串表示第 1 页。
// cfg.Random 为 true 时随机取 1..1000 页并返回 NextCursor 为空。
func (s *DanbooruSource) Fetch(ctx context.Context, cursor string) (Page, error) {
	limit := s.cfg.Limit
	if limit <= 0 {
		limit = 20
	}

	var page int
	random := s.cfg.Random
	if random {
		page = rand.IntN(danbooruRandomPageMax) + 1
	} else if cursor == "" {
		page = 1
	} else {
		p, err := strconv.Atoi(cursor)
		if err != nil {
			return Page{}, fmt.Errorf("parse danbooru page %q: %w", cursor, err)
		}
		page = p
	}

	tagList := make([]string, 0, len(s.cfg.Tags)+1)
	if prefix := danbooruRatingPrefix(s.cfg.Rating); prefix != "" {
		tagList = append(tagList, prefix)
	}
	tagList = append(tagList, s.cfg.Tags...)

	q := url.Values{
		"limit": {strconv.Itoa(limit)},
		"page":  {strconv.Itoa(page)},
	}
	if len(tagList) > 0 {
		q.Set("tags", strings.Join(tagList, " "))
	}
	rawURL := s.apiBase + "/posts.json?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, fmt.Errorf("danbooru request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("danbooru request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return Page{}, fmt.Errorf("danbooru read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("danbooru request: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var posts []danbooruPost
	if err := json.Unmarshal(body, &posts); err != nil {
		return Page{}, fmt.Errorf("danbooru parse response: %w", err)
	}

	works := make([]Work, 0, len(posts))
	for _, p := range posts {
		fileURL := p.FileURL
		if fileURL == "" {
			fileURL = p.LargeFileURL
		}
		if fileURL == "" {
			continue
		}
		tags := strings.Fields(p.TagString)
		works = append(works, Work{
			ID:        p.ID,
			Source:    "danbooru",
			Author:    p.UploaderName,
			Tags:      tags,
			PageCount: 1,
			AI:        hasAITag(tags),
			XRestrict: danbooruXRestrict(p.Rating),
			Pages:     []ImageURL{{Index: 0, URL: fileURL}},
		})
	}

	next := ""
	if !random && len(posts) == limit {
		next = strconv.Itoa(page + 1)
	}
	return Page{Works: works, NextCursor: next}, nil
}
