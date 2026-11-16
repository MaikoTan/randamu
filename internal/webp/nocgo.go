//go:build !cgo

package webp

import (
	"errors"
	"image"
	"io"
)

// DefaultQuality 是 WebP 编码质量（0-100），与 cgo 版本保持一致。
const DefaultQuality = 85

var errNoCGO = errors.New("webp: 二进制未以 cgo 构建，WebP 编解码不可用")

// Encode 在无 cgo 构建下始终失败，调用方会保留原图。
func Encode(io.Writer, image.Image, int) error { return errNoCGO }

// Decode 在无 cgo 构建下始终失败。
func Decode(io.Reader) (image.Image, error) { return nil, errNoCGO }

// DecodeConfig 在无 cgo 构建下始终失败。
func DecodeConfig(io.Reader) (image.Config, error) { return image.Config{}, errNoCGO }
