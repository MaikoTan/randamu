//go:build darwin && amd64

package webp

import _ "embed"

//go:embed lib/darwin_amd64.gz
var embeddedLib []byte

const libFileName = "libwebp_macos_amd64.dylib"
