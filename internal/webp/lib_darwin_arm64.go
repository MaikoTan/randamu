//go:build darwin && arm64

package webp

import _ "embed"

//go:embed lib/darwin_arm64.gz
var embeddedLib []byte

const libFileName = "libwebp_macos_arm64.dylib"
