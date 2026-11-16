//go:build windows

package webp

import (
	"fmt"
	"syscall"

	"github.com/ebitengine/purego"
)

// openLib 用 LoadLibrary 加载 DLL（purego.Dlopen 在 Windows 上不可用）。
func openLib(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}

// registerFn 取出进程地址后用 purego.RegisterFunc 绑定到 fptr。
func registerFn(fptr any, handle uintptr, name string) {
	addr, err := syscall.GetProcAddress(syscall.Handle(handle), name)
	if err != nil {
		panic(fmt.Sprintf("webp: 找不到符号 %s: %v", name, err))
	}
	purego.RegisterFunc(fptr, uintptr(addr))
}
