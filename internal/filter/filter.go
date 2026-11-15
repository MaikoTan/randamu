// Package filter implements work filtering rules matching the old fish script.
package filter

import (
	"strings"

	"github.com/MaikoTan/randamu/internal/pixiv"
	"github.com/MaikoTan/randamu/internal/source"
)

// Options are the filter options.
type Options struct {
	SkipAI       bool
	SkipR18      bool
	SkipR18G     bool
	MaxPageCount int
	Blacklist    []string
}

// SkipWork 判断一个图源无关的作品是否应跳过，返回原因。
// ugoira 与 pixiv 相同：kind/type 为 ugoira 时跳过。标签由各图源适配器填充，
// pixiv 源同时包含标签名与译名（黑名单按子串匹配两者）。
func SkipWork(w source.Work, o Options) (bool, string) {
	if w.Kind == "ugoira" {
		return true, "ugoira"
	}
	if o.SkipAI && w.AI {
		return true, "AI generated"
	}
	if o.SkipR18G && w.XRestrict > 1 {
		return true, "R18G"
	}
	if o.SkipR18 && w.XRestrict == 1 {
		return true, "R18"
	}
	for _, t := range w.Tags {
		for _, b := range o.Blacklist {
			if b == "" {
				continue
			}
			if containsFold(t, b) {
				return true, "blacklisted tag " + b
			}
		}
	}
	if o.MaxPageCount > 0 && w.PageCount > o.MaxPageCount {
		return true, "too many pages"
	}
	return false, ""
}

// Skip reports whether a work should be skipped and the reason.
func Skip(i pixiv.Illust, o Options) (bool, string) {
	if i.Type == "ugoira" {
		return true, "ugoira"
	}
	if o.SkipAI && i.IllustAIType == 2 {
		return true, "AI generated"
	}
	if o.SkipR18G && i.XRestrict > 1 {
		return true, "R18G"
	}
	if o.SkipR18 && i.XRestrict == 1 {
		return true, "R18"
	}
	// The old script used regex substring matching; keep case-insensitive substring matching.
	for _, t := range i.Tags {
		for _, b := range o.Blacklist {
			if b == "" {
				continue
			}
			if containsFold(t.Name, b) || containsFold(t.TranslatedName, b) {
				return true, "blacklisted tag " + b
			}
		}
	}
	if o.MaxPageCount > 0 && i.PageCount > o.MaxPageCount {
		return true, "too many pages"
	}
	return false, ""
}

// OriginalURLs returns the original image URL of every page.
func OriginalURLs(i pixiv.Illust) []string {
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

func containsFold(s, sub string) bool {
	if s == "" || sub == "" {
		return false
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
