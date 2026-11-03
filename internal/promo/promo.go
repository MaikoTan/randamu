// Package promo looks for promotional text (Fanbox, Fantia, ...) in a
// downloaded image by running the optional tesseract OCR binary. It never
// adds a build-time dependency: when tesseract is not in PATH the check is
// skipped and the image is kept.
package promo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultKeywords are the lowercase markers of a paid-content promotion.
// They are ASCII on purpose: OCR runs with the English model only.
var DefaultKeywords = []string{
	"fanbox", "fantia", "patreon", "dlsite", "gumroad",
	"booth.pm", "ci-en", "ko-fi", "afdian",
}

// ErrUnavailable reports that the tesseract binary is not in PATH.
var ErrUnavailable = errors.New("tesseract not found in PATH")

// Scan OCRs path and returns the first keyword found in the text, or "".
func Scan(path string, keywords []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// The default page-segmentation mode reads small watermarks more
	// reliably than sparse-text mode (measured on corner "FANBOX ONLY"
	// watermarks that sparse mode drops).
	cmd := exec.CommandContext(ctx, "tesseract", path, "stdout", "-l", "eng")
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrUnavailable
		}
		return "", fmt.Errorf("tesseract %s: %w", path, err)
	}
	return Match(string(out), keywords), nil
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
