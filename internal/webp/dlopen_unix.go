//go:build !windows

package webp

import "github.com/ebitengine/purego"

// openLib 用 dlopen 加载库文件。
func openLib(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

// registerFn 把已加载库中的符号绑定到 fptr。
func registerFn(fptr any, handle uintptr, name string) {
	purego.RegisterLibFunc(fptr, handle, name)
}
