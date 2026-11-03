package filter

import (
	"reflect"
	"testing"

	"github.com/MaikoTan/randamu/internal/pixiv"
)

func TestSkip(t *testing.T) {
	base := pixiv.Illust{ID: 1, Type: "illust", PageCount: 1}
	cases := []struct {
		name string
		mut  func(*pixiv.Illust)
		opts Options
		want bool
	}{
		{"regular work", func(*pixiv.Illust) {}, Options{}, false},
		{"ugoira", func(i *pixiv.Illust) { i.Type = "ugoira" }, Options{}, true},
		{"AI", func(i *pixiv.Illust) { i.IllustAIType = 2 }, Options{SkipAI: true}, true},
		{"AI but filter disabled", func(i *pixiv.Illust) { i.IllustAIType = 2 }, Options{}, false},
		{"R18", func(i *pixiv.Illust) { i.XRestrict = 1 }, Options{SkipR18: true}, true},
		{"R18 but filter disabled", func(i *pixiv.Illust) { i.XRestrict = 1 }, Options{}, false},
		{"R18G", func(i *pixiv.Illust) { i.XRestrict = 2 }, Options{SkipR18G: true}, true},
		{"blacklist substring", func(i *pixiv.Illust) {
			i.Tags = []pixiv.Tag{{Name: "3Dカスタム少女"}}
		}, Options{Blacklist: []string{"3D"}}, true},
		{"blacklist is case-insensitive", func(i *pixiv.Illust) {
			i.Tags = []pixiv.Tag{{Name: "MMD"}}
		}, Options{Blacklist: []string{"mmd"}}, true},
		{"blacklist matches translated name", func(i *pixiv.Illust) {
			i.Tags = []pixiv.Tag{{Name: "風景", TranslatedName: "landscape"}}
		}, Options{Blacklist: []string{"land"}}, true},
		{"too many pages", func(i *pixiv.Illust) { i.PageCount = 11 }, Options{MaxPageCount: 10}, true},
		{"exactly at page limit", func(i *pixiv.Illust) { i.PageCount = 10 }, Options{MaxPageCount: 10}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := base
			tc.mut(&i)
			got, reason := Skip(i, tc.opts)
			if got != tc.want {
				t.Fatalf("Skip = %v (%s), want %v", got, reason, tc.want)
			}
		})
	}
}

func TestOriginalURLs(t *testing.T) {
	single := pixiv.Illust{
		PageCount:      1,
		MetaSinglePage: pixiv.SinglePage{OriginalImageURL: "https://i.pximg.net/single.jpg"},
	}
	if got := OriginalURLs(single); !reflect.DeepEqual(got, []string{"https://i.pximg.net/single.jpg"}) {
		t.Fatalf("single page: %v", got)
	}

	fallback := pixiv.Illust{PageCount: 1, ImageURLs: map[string]string{"original": "https://i.pximg.net/fallback.jpg"}}
	if got := OriginalURLs(fallback); !reflect.DeepEqual(got, []string{"https://i.pximg.net/fallback.jpg"}) {
		t.Fatalf("fallback: %v", got)
	}

	multi := pixiv.Illust{
		PageCount: 2,
		MetaPages: []pixiv.MetaPage{
			{ImageURLs: map[string]string{"original": "a"}},
			{ImageURLs: map[string]string{"original": "b"}},
		},
	}
	if got := OriginalURLs(multi); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("multi page: %v", got)
	}

	if got := OriginalURLs(pixiv.Illust{PageCount: 1}); len(got) != 0 {
		t.Fatalf("no URL: %v", got)
	}
}
