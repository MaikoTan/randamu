// Package promo looks for promotional text (Fanbox, Fantia, ...) in a
// downloaded image by running the optional tesseract OCR binary. It never
// adds a build-time dependency: when tesseract is neither in PATH nor next
// to the randamu binary, the check is skipped and the image is kept.
package promo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultKeywords are the lowercase markers of a paid-content promotion.
// They are ASCII on purpose: OCR runs with the English model only.
var DefaultKeywords = []string{
	"fanbox", "fantia", "patreon", "dlsite", "gumroad",
	"booth.pm", "ci-en", "ko-fi", "afdian",
}

// ErrUnavailable reports that the tesseract binary is neither in PATH nor
// bundled next to the randamu binary.
var ErrUnavailable = errors.New("tesseract not found")

// Scan OCRs path and returns the first keyword found in the text, or "".
func Scan(path string, keywords []string) (string, error) {
	binary, args, err := locate()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// The default page-segmentation mode reads small watermarks more
	// reliably than sparse-text mode (measured on corner "FANBOX ONLY"
	// watermarks that sparse mode drops).
	cmd := exec.CommandContext(ctx, binary, append(args, path, "stdout", "-l", "eng")...)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrUnavailable
		}
		return "", fmt.Errorf("tesseract %s: %w", path, err)
	}
	return Match(string(out), keywords), nil
}

// locate finds the tesseract binary. It prefers PATH and falls back to the
// bundled copy in <executable dir>/tesseract (shipped in the Windows release
// archives). A bundled copy gets its own tessdata directory, which may hold
// a different language set than an installed tesseract.
func locate() (binary string, args []string, err error) {
	if p, e := exec.LookPath("tesseract"); e == nil {
		return p, nil, nil
	}
	dir, e := os.Executable()
	if e != nil {
		return "", nil, ErrUnavailable
	}
	return locateFrom(filepath.Dir(dir))
}

// locateFrom is locate with the executable directory injected for tests.
func locateFrom(exeDir string) (binary string, args []string, err error) {
	bundled := filepath.Join(exeDir, "tesseract", "tesseract.exe")
	if _, e := os.Stat(bundled); e != nil {
		return "", nil, ErrUnavailable
	}
	// Windows resolves DLLs from the exe's own directory, so no PATH change
	// is needed; the tessdata directory is passed explicitly because the
	// bundled copy does not rely on a system-wide installation prefix.
	return bundled, []string{"--tessdata-dir", filepath.Join(exeDir, "tesseract", "tessdata")}, nil
}

// Match returns the first keyword contained in text, case-insensitively.
func Match(text string, keywords []string) string {
	lower := strings.ToLower(text)
	for _, k := range keywords {
		if k != "" && strings.Contains(lower, strings.ToLower(k)) {
			return k
		}
	}
	return ""
}
