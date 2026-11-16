//go:build cgo

// Package webp 封装 WebP 的编解码，通过 cgo 静态链接 libwebp。
//
// libwebp 以静态库形式链接进二进制（nix 构建与发布流水线用 zig 交叉编译出
// 各目标平台的 libwebp.a），所以产物仍是单个可执行文件，运行时不需要系统
// 安装 libwebp，也不依赖 glibc 还是 musl。若以 CGO_ENABLED=0 构建，本包
// 退化为返回错误的桩（见 nocgo.go），下载流程会保留原图继续。
//
// 本包同时为内容分析（analyze）与变体合并（variation）提供 WebP 解码
// 能力，否则 Go 标准库无法读取 WebP。
package webp

/*
#cgo LDFLAGS: -lwebp -lsharpyuv -lm
#include <webp/encode.h>
#include <webp/decode.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"runtime"
	"unsafe"
)

// DefaultQuality 是 WebP 编码质量（0-100），越大质量越好、文件越大。
const DefaultQuality = 85

// Encode 将 img 编码为 WebP 写入 w，quality <= 0 表示使用默认值 85。
func Encode(w io.Writer, img image.Image, quality int) error {
	if quality <= 0 {
		quality = DefaultQuality
	}
	if quality > 100 {
		quality = 100
	}

	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	var out *C.uint8_t
	var size C.size_t
	if rgba.Rect.Dx() > 0 && rgba.Rect.Dy() > 0 && len(rgba.Pix) > 0 {
		size = C.WebPEncodeRGBA(
			(*C.uint8_t)(unsafe.Pointer(&rgba.Pix[0])),
			C.int(rgba.Rect.Dx()), C.int(rgba.Rect.Dy()),
			C.int(rgba.Stride), C.float(quality), &out)
	}
	runtime.KeepAlive(rgba)
	if size == 0 || out == nil {
		return errors.New("webp: 编码失败（libwebp 返回空）")
	}
	encoded := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(size))
	n, err := w.Write(encoded)
	C.WebPFree(unsafe.Pointer(out))
	if err != nil {
		return err
	}
	if n != len(encoded) {
		return io.ErrShortWrite
	}
	return nil
}

// Decode 从 r 中解码一张 WebP 图片。
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("webp: 解码失败（数据为空）")
	}

	var w, h C.int
	buf := C.WebPDecodeRGBA(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &w, &h)
	runtime.KeepAlive(data)
	if buf == nil {
		return nil, errors.New("webp: 解码失败（libwebp 返回空）")
	}
	if w <= 0 || h <= 0 {
		C.WebPFree(unsafe.Pointer(buf))
		return nil, fmt.Errorf("webp: 解码得到非法尺寸 %dx%d", int(w), int(h))
	}

	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	copy(img.Pix, unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(w)*int(h)*4))
	C.WebPFree(unsafe.Pointer(buf))
	return img, nil
}

// DecodeConfig 只解码 WebP 的图片配置（尺寸等），不分配像素。
func DecodeConfig(r io.Reader) (image.Config, error) {
	var cfg image.Config
	data, err := io.ReadAll(r)
	if err != nil {
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, errors.New("webp: 读取配置失败（数据为空）")
	}

	var w, h C.int
	ok := C.WebPGetInfo(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &w, &h)
	runtime.KeepAlive(data)
	if ok == 0 || w <= 0 || h <= 0 {
		return cfg, errors.New("webp: 读取配置失败（非 WebP 数据或已损坏）")
	}
	return image.Config{Width: int(w), Height: int(h), ColorModel: color.RGBAModel}, nil
}
