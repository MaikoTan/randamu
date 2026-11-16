//go:build linux

package webp

// libCanUnlink 为 true：Linux 遵循 POSIX 语义，dlopen 后 unlink 已加载的
// 库不影响既有映射，临时文件可立即清理。
const libCanUnlink = true
