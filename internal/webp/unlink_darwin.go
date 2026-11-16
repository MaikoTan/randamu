//go:build darwin

package webp

// libCanUnlink 为 false：macOS 不允许删除正在使用的 dylib，
// 临时文件会保留在系统临时目录中。
const libCanUnlink = false
