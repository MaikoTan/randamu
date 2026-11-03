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
		{"unknown", []Source{{Type: "lolicon"}}, true},
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
