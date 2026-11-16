//go:build windows && arm64

package webp

import _ "embed"

//go:embed lib/windows_arm64.gz
var embeddedLib []byte

const libFileName = "libwebp_windows_arm64.dll"
