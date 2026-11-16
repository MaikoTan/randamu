//go:build windows

package webp

// libCanUnlink 为 false：Windows 不允许删除正在使用的 DLL，
// 临时文件会保留在系统临时目录中。
const libCanUnlink = false
