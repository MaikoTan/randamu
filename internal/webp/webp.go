// Package webp 封装 WebP 的编解码，纯 Go，无 CGO。
//
// 编解码通过 github.com/ebitengine/purego 在运行时 dlopen 内嵌的 libwebp
// 共享库完成：每个目标平台的库文件经 gzip 压缩后通过 //go:embed 打包进
// 二进制（见各 lib_GOOS_GOARCH.go），首次使用时解压到临时目录再加载，
// 因此只需单个静态二进制即可脱离本地 libwebp 安装运行。
//
// 本包同时为内容分析（analyze）与变体合并（variation）提供 WebP 解码
// 能力，否则 Go 标准库无法读取 WebP。
package webp

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"
)

// DefaultQuality 是 WebP 编码质量（0-100），越大质量越好、文件越大。
const DefaultQuality = 85

// libwebp 的函数签名。size_t 在 32/64 位下宽度不同，统一用 uintptr 承接。
type (
	encodeRGBAFn  func(rgba *uint8, w, h, stride int, quality float32, output **uint8) uintptr
	webpFreeFn    func(ptr unsafe.Pointer)
	decodeRGBAFn  func(data *uint8, dataLen uintptr, w, h *int) *uint8
	webpGetInfoFn func(data *uint8, dataLen uintptr, w, h *int) int
)

var (
	libOnce sync.Once
	libErr  error

	libHandle uintptr

	webpEncodeRGBA         encodeRGBAFn
	webpEncodeLosslessRGBA encodeRGBAFn
	webpFree               webpFreeFn
	webpDecodeRGBA         decodeRGBAFn
	webpGetInfo            webpGetInfoFn
)

// loadLib 惰性加载内嵌的 libwebp：解压到临时目录 → dlopen → 注册符号。
func loadLib() error {
	libOnce.Do(func() {
		libErr = loadLibOnce()
	})
	return libErr
}

func loadLibOnce() error {
	dir, err := os.MkdirTemp("", "randamu-webp-*")
	if err != nil {
		return fmt.Errorf("webp: 创建临时目录失败: %w", err)
	}
	path := filepath.Join(dir, libFileName)

	zr, err := gzip.NewReader(bytes.NewReader(embeddedLib))
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("webp: 解压内嵌库失败: %w", err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("webp: 解压内嵌库失败: %w", err)
	}
	zr.Close()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("webp: 写入临时库文件失败: %w", err)
	}

	handle, err := openLib(path)
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("webp: 加载内嵌库失败: %w", err)
	}

	// Linux 允许在 dlopen 后立即 unlink 已加载的库，映射仍然有效，且 tmpfs
	// 上不留下任何文件。macOS 与 Windows 不允许删除正在使用的共享库，只能
	// 保留临时文件由系统临时目录清理。
	if libCanUnlink {
		os.Remove(path)
		os.Remove(dir)
	}

	registerFn(&webpEncodeRGBA, handle, "WebPEncodeRGBA")
	registerFn(&webpEncodeLosslessRGBA, handle, "WebPEncodeLosslessRGBA")
	registerFn(&webpFree, handle, "WebPFree")
	registerFn(&webpDecodeRGBA, handle, "WebPDecodeRGBA")
	registerFn(&webpGetInfo, handle, "WebPGetInfo")

	libHandle = handle
	return nil
}

// Encode 将 img 编码为 WebP 写入 w，quality <= 0 表示使用默认值 85。
func Encode(w io.Writer, img image.Image, quality int) error {
	if err := loadLib(); err != nil {
		return err
	}
	if quality <= 0 {
		quality = DefaultQuality
	}
	if quality > 100 {
		quality = 100
	}

	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	var out *uint8
	size := webpEncodeRGBA(&rgba.Pix[0], rgba.Rect.Dx(), rgba.Rect.Dy(), rgba.Stride, float32(quality), &out)
	runtime.KeepAlive(rgba)
	if size == 0 || out == nil {
		return errors.New("webp: 编码失败（libwebp 返回空）")
	}
	encoded := unsafe.Slice(out, int(size))
	n, err := w.Write(encoded)
	webpFree(unsafe.Pointer(out))
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
	if err := loadLib(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("webp: 解码失败（数据为空）")
	}
	pix := &data[0]

	var w, h int
	buf := webpDecodeRGBA(pix, uintptr(len(data)), &w, &h)
	runtime.KeepAlive(data)
	if buf == nil {
		return nil, errors.New("webp: 解码失败（libwebp 返回空）")
	}
	if w <= 0 || h <= 0 {
		webpFree(unsafe.Pointer(buf))
		return nil, fmt.Errorf("webp: 解码得到非法尺寸 %dx%d", w, h)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, unsafe.Slice(buf, w*h*4))
	webpFree(unsafe.Pointer(buf))
	return img, nil
}

// DecodeConfig 只解码 WebP 的图片配置（尺寸等），不分配像素。
func DecodeConfig(r io.Reader) (image.Config, error) {
	var cfg image.Config
	if err := loadLib(); err != nil {
		return cfg, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return cfg, err
	}
	if len(data) == 0 {
		return cfg, errors.New("webp: 读取配置失败（数据为空）")
	}
	pix := &data[0]

	var w, h int
	ok := webpGetInfo(pix, uintptr(len(data)), &w, &h)
	runtime.KeepAlive(data)
	if ok == 0 || w <= 0 || h <= 0 {
		return cfg, errors.New("webp: 读取配置失败（非 WebP 数据或已损坏）")
	}
	return image.Config{Width: w, Height: h, ColorModel: color.RGBAModel}, nil
}
