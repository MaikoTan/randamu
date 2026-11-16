//go:build windows && amd64

package webp

import _ "embed"

//go:embed lib/windows_amd64.gz
var embeddedLib []byte

const libFileName = "libwebp_windows_amd64.dll"
