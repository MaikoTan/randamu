package source

import (
	"context"
	"fmt"
	"strconv"

	"github.com/MaikoTan/randamu/internal/config"
	"github.com/MaikoTan/randamu/internal/pixiv"
)

// PixivSource 把 pixiv 客户端包装成 Source。
type PixivSource struct {
	client *pixiv.Client
	kind   string // bookmarks / search / recommended
	cfg    config.Source
	userID int64
	// Viewed 返回最近已看过的作品 ID，供 recommended 去重；可为 nil。
	Viewed func() []int64
	// RecordViewed 记录一个已看过的作品 ID；可为 nil。
	RecordViewed func(id int64)
}

// NewPixiv 构造 pixiv 图源适配器。kind 为 bookmarks/search/recommended。
func NewPixiv(client *pixiv.Client, kind string, cfg config.Source, userID int64) *PixivSource {
	return &PixivSource{client: client, kind: kind, cfg: cfg, userID: userID}
}

// Fetch 抓取一页作品。
// bookmarks/search 的 cursor 就是 next_url 原样透传；
// recommended 的 cursor 是十进制偏移量。
func (s *PixivSource) Fetch(ctx context.Context, cursor string) (Page, error) {
	var p pixiv.Page
	var err error
	switch s.kind {
	case "bookmarks":
		p, err = s.client.BookmarksPage(ctx, s.userID, s.cfg.Restrict, cursor)
	case "search":
		p, err = s.client.SearchPage(ctx, pixiv.SearchParams{
			Word:         s.cfg.Tag,
			SearchTarget: s.cfg.SearchTarget,
			Sort:         s.cfg.Sort,
			Duration:     s.cfg.Duration,
		}, cursor)
	case "recommended":
		offset := 0
		if cursor != "" {
			offset, err = strconv.Atoi(cursor)
			if err != nil {
				return Page{}, fmt.Errorf("parse recommended offset %q: %w", cursor, err)
			}
		}
		p, err = s.client.RecommendedPage(ctx, offset, s.viewed())
		if err != nil {
			return Page{}, err
		}
		if s.RecordViewed != nil {
			for _, i := range p.Illusts {
				s.RecordViewed(i.ID)
			}
		}
		return Page{Works: mapIllusts(p.Illusts), NextCursor: strconv.Itoa(offset + len(p.Illusts))}, nil
	default:
		return Page{}, fmt.Errorf("unknown pixiv source kind %q", s.kind)
	}
	if err != nil {
		return Page{}, err
	}
	return Page{Works: mapIllusts(p.Illusts), NextCursor: p.NextURL}, nil
}

// viewed 返回最近已看过的作品 ID，nil 回调时返回空。
func (s *PixivSource) viewed() []int64 {
	if s.Viewed == nil {
		return nil
	}
	return s.Viewed()
}

// mapIllusts 把 pixiv 作品列表转换为与图源无关的 Work 列表。
func mapIllusts(illusts []pixiv.Illust) []Work {
	works := make([]Work, 0, len(illusts))
	for _, i := range illusts {
		tags := make([]string, 0, len(i.Tags))
		for _, t := range i.Tags {
			tags = append(tags, t.Name)
		}
		urls := originalURLs(i)
		pages := make([]ImageURL, 0, len(urls))
		for idx, u := range urls {
			pages = append(pages, ImageURL{Index: idx, URL: u})
		}
		works = append(works, Work{
			ID:        i.ID,
			Source:    "pixiv",
			Kind:      i.Type,
			Title:     i.Title,
			Author:    i.User.Name,
			AuthorID:  i.User.ID,
			Tags:      tags,
			PageCount: i.PageCount,
			AI:        i.IllustAIType == 2,
			XRestrict: i.XRestrict,
			Pages:     pages,
		})
	}
	return works
}

// originalURLs 提取每页原图地址，逻辑与 filter.OriginalURLs 一致。
// 这里刻意复制而不导入 internal/filter，以避免包依赖环。
func originalURLs(i pixiv.Illust) []string {
	if i.PageCount > 1 {
		urls := make([]string, 0, len(i.MetaPages))
		for _, p := range i.MetaPages {
			if u := p.ImageURLs["original"]; u != "" {
				urls = append(urls, u)
			}
		}
		return urls
	}
	if u := i.MetaSinglePage.OriginalImageURL; u != "" {
		return []string{u}
	}
	if u := i.ImageURLs["original"]; u != "" {
		return []string{u}
	}
	return nil
}
