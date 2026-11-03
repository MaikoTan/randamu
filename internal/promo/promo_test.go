package promo

import (
	"errors"
	"os/exec"
	"testing"
)

func TestMatch(t *testing.T) {
	kws := []string{"fanbox", "fantia", "patreon"}
	cases := []struct {
		text string
		want string
	}{
		{"https://kataokasan.fanbox.cc/posts/10884454", "fanbox"},
		{"FAN BOX", ""}, // the space breaks the word; a bare logo is not matched
		{"FANBOX", "fanbox"},
		{"続きはFantiaで", "fantia"},
		{"no promo here", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := Match(c.text, kws); got != c.want {
			t.Errorf("Match(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestScanMissingBinary(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err == nil {
		t.Skip("tesseract is installed")
	}
	if _, err := Scan("whatever.png", nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Scan error = %v, want ErrUnavailable", err)
	}
}
