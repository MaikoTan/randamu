// Package webp 封装 WebP 的编解码，纯 Go，无 CGO。
//
// 编解码统一使用 github.com/gen2brain/webp（libwebp 的纯 Go 移植，
// 经 purego 调用，兼容 CGO_ENABLED=0）。chai2010/webp 依赖 C 库
// libwebp，在 CGO_ENABLED=0 下无法编译，已弃用。
// 本包同时为内容分析（analyze）与变体合并（variation）提供 WebP
// 解码能力，否则 Go 标准库无法读取 WebP。
package webp

import (
	"image"
	"io"

	"github.com/gen2brain/webp"
)

// DefaultQuality 是 WebP 编码质量（0-100），越大质量越好、文件越大。
const DefaultQuality = 85

// Encode 将 img 编码为 WebP 写入 w，quality 为 0 表示使用默认值 85。
func Encode(w io.Writer, img image.Image, quality int) error {
	if quality <= 0 || quality > 100 {
		quality = DefaultQuality
	}
	options := webp.Options{Lossless: false, Quality: quality}
	return webp.Encode(w, img, options)
}

// Decode 从 r 中解码一张 WebP 图片。
func Decode(r io.Reader) (image.Image, error) {
	return webp.Decode(r)
}

// DecodeConfig 只解码 WebP 的图片配置（尺寸等），不分配像素。
func DecodeConfig(r io.Reader) (image.Config, error) {
	return webp.DecodeConfig(r)
}
