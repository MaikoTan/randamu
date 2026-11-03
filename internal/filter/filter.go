// Package filter implements work filtering rules matching the old fish script.
package filter

import (
	"strings"

	"github.com/MaikoTan/randamu/internal/pixiv"
)

// Options are the filter options.
type Options struct {
	SkipAI       bool
	SkipR18      bool
	SkipR18G     bool
	MaxPageCount int
	Blacklist    []string
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
