package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingImplicitUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.json"), false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Concurrency != 4 || len(cfg.Sources) == 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadMissingExplicitFails(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json"), true); err == nil {
		t.Fatal("an explicitly specified missing file must fail")
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"concurrency": 8, "skip_ai": false, "sources": [{"type":"search","tag":"風景"}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Concurrency != 8 || cfg.SkipAI {
		t.Fatalf("file values not applied: %+v", cfg)
	}
	// Fields absent from the file must keep their defaults.
	if cfg.MaxPageCount != 10 || cfg.MaxRetries != 3 {
		t.Fatalf("defaults were unexpectedly overridden: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Sources[0].SearchTarget != "exact_match_for_tags" || cfg.Sources[0].Sort != "popular_desc" {
		t.Fatalf("search defaults not filled: %+v", cfg.Sources[0])
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, true); err == nil {
		t.Fatal("invalid JSON must fail")
	}
}

func TestContentAnalysisDefaults(t *testing.T) {
	cfg := Default()
	cfg.QRMinCoverage = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !cfg.SkipQR || !cfg.SkipMosaic || !cfg.SkipBlur || cfg.QRMinCoverage != 0.02 {
		t.Fatalf("content analysis defaults wrong: qr=%v mosaic=%v blur=%v coverage=%v",
			cfg.SkipQR, cfg.SkipMosaic, cfg.SkipBlur, cfg.QRMinCoverage)
	}
}

func TestVariationDefaults(t *testing.T) {
	cfg := Default()
	cfg.VariationMinSimilarity = 0
	cfg.VariationDelayMS = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !cfg.MergeVariations || cfg.VariationMinSimilarity != 0.9 || cfg.VariationDelayMS != 1000 {
		t.Fatalf("variation defaults not restored: %+v",
			[]any{cfg.MergeVariations, cfg.VariationMinSimilarity, cfg.VariationDelayMS})
	}
}

func TestPromoDefaults(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !cfg.SkipPromoText {
		t.Fatal("skip_promo_text must be enabled by default")
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("RANDAMU_IMAGES_DIR", "/tmp/images")
	t.Setenv("RANDAMU_STATE_DIR", "/tmp/state")
	t.Setenv("RANDAMU_PROXY", "http://127.0.0.1:7890")

	cfg := Default()
	cfg.ApplyEnv()
	if cfg.ImagesDir != "/tmp/images" || cfg.StateDir != "/tmp/state" || cfg.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("environment variables not applied: %+v", cfg)
	}
}

func TestValidateSources(t *testing.T) {
	cases := []struct {
		name    string
		sources []Source
		wantErr bool
	}{
		{"bookmarks default restrict", []Source{{Type: "bookmarks"}}, false},
		{"bookmarks invalid restrict", []Source{{Type: "bookmarks", Restrict: "secret"}}, true},
		{"search without tag", []Source{{Type: "search"}}, true},
		{"lolicon", []Source{{Type: "lolicon"}}, false},
		{"danbooru without tags", []Source{{Type: "danbooru"}}, false},
		{"konachan rating", []Source{{Type: "konachan", Rating: "questionable"}}, false},
		{"konachan bad rating", []Source{{Type: "konachan", Rating: "nsfw"}}, true},
		{"unknown", []Source{{Type: "nope"}}, true},
		{"recommended", []Source{{Type: "recommended"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Sources = tc.sources
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if err == nil && cfg.Sources[0].Limit != 100 {
				t.Fatalf("limit default not applied: %d", cfg.Sources[0].Limit)
			}
		})
	}
}

func TestWebPQualityClamp(t *testing.T) {
	for _, q := range []int{-5, 0, 101, 1000} {
		cfg := Default()
		cfg.WebPQuality = q
		if err := cfg.Validate(); err != nil {
			t.Fatalf("quality %d: Validate: %v", q, err)
		}
		if cfg.WebPQuality != 85 {
			t.Fatalf("quality %d: clamp = %d, want 85", q, cfg.WebPQuality)
		}
	}
	cfg := Default()
	cfg.WebPQuality = 70
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.WebPQuality != 70 {
		t.Fatalf("valid quality changed to %d", cfg.WebPQuality)
	}
}

func TestApplyEnvWebP(t *testing.T) {
	cases := map[string]bool{
		"off": false, "0": false, "false": false, "no": false, "OFF": false,
		"on": true, "1": true, "true": true, "yes": true, "on ": true,
	}
	for v, want := range cases {
		t.Setenv("RANDAMU_WEBP", v)
		cfg := Default()
		cfg.WebP = !want // 先置反，确认环境变量确实生效
		cfg.ApplyEnv()
		if cfg.WebP != want {
			t.Fatalf("RANDAMU_WEBP=%q: webp = %v, want %v", v, cfg.WebP, want)
		}
	}
	// 未识别的值不改变配置
	t.Setenv("RANDAMU_WEBP", "maybe")
	cfg := Default()
	cfg.WebP = true
	cfg.ApplyEnv()
	if !cfg.WebP {
		t.Fatal("unrecognised RANDAMU_WEBP changed the setting")
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Default()
	cfg.ImagesDir = "~/pics"
	cfg.StateDir = home + "/state"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.ImagesDir != filepath.Join(home, "pics") {
		t.Fatalf("~ not expanded: %s", cfg.ImagesDir)
	}
}
