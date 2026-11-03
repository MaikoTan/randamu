package promo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func TestScanBundledBinary(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err == nil {
		t.Skip("tesseract is installed")
	}
	// A stub next to the (virtual) randamu binary stands in for the
	// bundled tesseract shipped in the Windows release archives.
	dir := t.TempDir()
	bundled := filepath.Join(dir, "tesseract")
	if err := os.MkdirAll(bundled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundled, "tesseract.exe"), []byte{0}, 0o755); err != nil {
		t.Fatal(err)
	}
	got, args, err := locateFrom(dir)
	if err != nil {
		t.Fatalf("locateFrom error = %v, want the bundled binary", err)
	}
	if got != filepath.Join(bundled, "tesseract.exe") {
		t.Errorf("locateFrom = %q, want %q", got, bundled+string(filepath.Separator)+"tesseract.exe")
	}
	want := []string{"--tessdata-dir", filepath.Join(bundled, "tessdata")}
	if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
		t.Errorf("locateFrom args = %v, want %v", args, want)
	}
	// Without the bundle directory, lookup fails.
	if _, _, err := locateFrom(t.TempDir()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("locateFrom error = %v, want ErrUnavailable", err)
	}
}
