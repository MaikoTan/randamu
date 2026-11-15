// Package source defines a source-agnostic work abstraction and the adapters
// that fetch works from each image source.
package source

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/MaikoTan/randamu/internal/config"
	"github.com/MaikoTan/randamu/internal/pixiv"
)

// Work 是图源无关的作品描述。
type Work struct {
	ID        int64  // 图源内作品 ID（pixiv illust id / danbooru post id / ...）
	Source    string // 图源名：pixiv / lolicon / konachan / danbooru
	Kind      string // 作品类型，如 pixiv 的 illust/ugoira/manga
	Title     string
	Author    string // 作者名
	AuthorID  int64
	Tags      []string
	PageCount int
	AI        bool // AI 生成
	XRestrict int  // 0 全年龄, 1 R18, 2 R18G（与 pixiv x_restrict 一致）
	Pages     []ImageURL
}

// ImageURL 是作品某一页的下载地址。
type ImageURL struct {
	Index int    // 第几页，从 0 开始
	URL   string // 下载地址（尽量给原图/最高分辨率）
}

// Page 是一次抓取的结果。
type Page struct {
	Works      []Work
	NextCursor string // 空字符串表示没有下一页
}

// maxBodySize 是 JSON 响应体的大小上限。
const maxBodySize = 10 << 20

// truncate 截断过长的错误信息。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Source 是一个图源适配器。Cursor 语义由各实现自定（可为空串表示从头开始）。
type Source interface {
	Fetch(ctx context.Context, cursor string) (Page, error)
}

// Options 汇总构造图源所需的依赖。
type Options struct {
	Pixiv     *pixiv.Client // 可为 nil（仅当不含 pixiv 源时）
	HTTP      *http.Client
	UserAgent string
	Logger    *slog.Logger
}

// New 按 cfg.Type 构造对应的图源适配器。
func New(cfg config.Source, o Options) (Source, error) {
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	switch cfg.Type {
	case "bookmarks", "search", "recommended":
		if o.Pixiv == nil {
			return nil, fmt.Errorf("source %q requires a pixiv client", cfg.Type)
		}
		// New 不携带登录用户 ID；需要书签源时请直接调用 NewPixiv 并传入 userID。
		return NewPixiv(o.Pixiv, cfg.Type, cfg, 0), nil
	case "lolicon":
		return NewLolicon(cfg, o), nil
	case "konachan":
		return NewKonachan(cfg, o), nil
	case "danbooru":
		return NewDanbooru(cfg, o), nil
	default:
		return nil, fmt.Errorf("unknown source type %q", cfg.Type)
	}
}
