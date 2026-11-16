//go:build linux && amd64

package webp

import _ "embed"

//go:embed lib/linux_amd64.gz
var embeddedLib []byte

const libFileName = "libwebp_linux_amd64.so"
