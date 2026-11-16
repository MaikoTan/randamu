//go:build linux && arm

package webp

import _ "embed"

//go:embed lib/linux_arm.gz
var embeddedLib []byte

const libFileName = "libwebp_linux_arm.so"
