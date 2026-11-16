//go:build linux && 386

package webp

import _ "embed"

//go:embed lib/linux_386.gz
var embeddedLib []byte

const libFileName = "libwebp_linux_386.so"
