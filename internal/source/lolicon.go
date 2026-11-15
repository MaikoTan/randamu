package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/MaikoTan/randamu/internal/config"
)

// defaultLoliconAPIBase 是 lolicon v2 接口的默认地址。
const defaultLoliconAPIBase = "https://api.lolicon.app/setu/v2"

// loliconMaxNum 是单次请求的作品数上限（API 限制）。
const loliconMaxNum = 20

// LoliconSource 是 lolicon v2 图源适配器。
type LoliconSource struct {
	cfg     config.Source
	http    *http.Client
	apiBase string
}

// NewLolicon 构造 lolicon 图源适配器。
func NewLolicon(cfg config.Source, o Options) *LoliconSource {
	base := cfg.APIBase
	if base == "" {
		base = defaultLoliconAPIBase
	}
	return &LoliconSource{cfg: cfg, http: o.HTTP, apiBase: base}
}

// loliconRequest 是 lolicon v2 的请求体。
type loliconRequest struct {
	Tag       []string `json:"tag,omitempty"`
	R18       int      `json:"r18"`
	Num       int      `json:"num"`
	ExcludeAI bool     `json:"excludeAI"`
	Proxy     string   `json:"proxy"`
	Size      []string `json:"size"`
}

// loliconResponse 是 lolicon v2 的响应体。
type loliconResponse struct {
	Code int             `json:"code"`
	Data []loliconIllust `json:"data"`
}

type loliconIllust struct {
	PID    int64    `json:"pid"`
	UID    int64    `json:"uid"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Tags   []string `json:"tags"`
	R18    bool     `json:"r18"`
	URLs   struct {
		Regular  string `json:"regular"`
		Original string `json:"original"`
	} `json:"urls"`
}

// loliconR18 把配置的 rating 映射为 lolicon 的 r18 参数：0 非R18、1 仅R18、2 混合。
// 未指定或未知值一律按非R18 处理，保守起见。
func loliconR18(rating string) int {
	switch rating {
	case "r18":
		return 1
	default:
		return 0
	}
}

// Fetch 抓取一批随机作品。lolicon 无分页，返回的单批结果即为全部，NextCursor 为空。
func (s *LoliconSource) Fetch(ctx context.Context, cursor string) (Page, error) {
	tags := s.cfg.Tags
	if len(tags) == 0 && s.cfg.Tag != "" {
		tags = []string{s.cfg.Tag}
	}
	num := s.cfg.Limit
	if num <= 0 || num > loliconMaxNum {
		num = loliconMaxNum
	}

	reqBody := loliconRequest{
		Tag:       tags,
		R18:       loliconR18(s.cfg.Rating),
		Num:       num,
		ExcludeAI: true, // 与 pixiv SkipAI 默认一致，过滤 AI 图
		Proxy:     "null",
		Size:      []string{"regular", "original"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Page{}, fmt.Errorf("lolicon encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBase, bytes.NewReader(payload))
	if err != nil {
		return Page{}, fmt.Errorf("lolicon request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("lolicon request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return Page{}, fmt.Errorf("lolicon read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("lolicon request: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var r loliconResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Page{}, fmt.Errorf("lolicon parse response: %w", err)
	}
	if r.Code != 0 {
		return Page{}, fmt.Errorf("lolicon returned code %d", r.Code)
	}

	works := make([]Work, 0, len(r.Data))
	for _, d := range r.Data {
		url := d.URLs.Original
		if url == "" {
			url = d.URLs.Regular
		}
		if url == "" {
			continue
		}
		xr := 0
		if d.R18 {
			xr = 1
		}
		works = append(works, Work{
			ID:        d.PID,
			Source:    "lolicon",
			Title:     d.Title,
			Author:    d.Author,
			AuthorID:  d.UID,
			Tags:      d.Tags,
			PageCount: 1,
			AI:        false,
			XRestrict: xr,
			Pages:     []ImageURL{{Index: 0, URL: url}},
		})
	}
	return Page{Works: works, NextCursor: ""}, nil
}
