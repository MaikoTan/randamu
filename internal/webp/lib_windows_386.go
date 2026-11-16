//go:build windows && 386

package webp

import _ "embed"

//go:embed lib/windows_386.gz
var embeddedLib []byte

const libFileName = "libwebp_windows_386.dll"
