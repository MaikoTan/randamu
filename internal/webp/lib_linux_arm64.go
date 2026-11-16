//go:build linux && arm64

package webp

import _ "embed"

//go:embed lib/linux_arm64.gz
var embeddedLib []byte

const libFileName = "libwebp_linux_arm64.so"
