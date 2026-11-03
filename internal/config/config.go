// Package config loads, merges and validates randamu configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source describes one fetch source.
type Source struct {
	// Type is one of bookmarks, search or recommended.
	Type string `json:"type"`
	// Restrict is only used by bookmarks and is either public or private.
	Restrict string `json:"restrict,omitempty"`
	// Tag is only used by search.
	Tag          string `json:"tag,omitempty"`
	SearchTarget string `json:"search_target,omitempty"`
	Sort         string `json:"sort,omitempty"`
	Duration     string `json:"duration,omitempty"`
	// Limit is the maximum number of works handled per run for this source.
	Limit int `json:"limit,omitempty"`
}

// Config is the complete program configuration.
type Config struct {
	ImagesDir    string `json:"images_dir"`
	StateDir     string `json:"state_dir"`
	Proxy        string `json:"proxy"`
	UserID       int64  `json:"user_id"`
	Concurrency  int    `json:"concurrency"`
	MaxRetries   int    `json:"max_retries"`
	MaxPageCount int    `json:"max_page_count"`
	SkipAI       bool   `json:"skip_ai"`
	SkipR18      bool   `json:"skip_r18"`
	SkipR18G     bool   `json:"skip_r18g"`
	// SkipQR, SkipMosaic and SkipBlur reject non-illustration images by
	// content analysis after download: QR-code posters, mosaic-censored
	// works and fully blurred paid-site teasers.
	SkipQR     bool `json:"skip_qr"`
	SkipMosaic bool `json:"skip_mosaic"`
	SkipBlur   bool `json:"skip_blur"`
	// QRMinCoverage is the share of the image a QR code must cover before
	// the image is rejected (default 0.02).
	QRMinCoverage float64 `json:"qr_min_coverage"`
	// MergeVariations combines the pages of one work into a single animated
	// PNG when they look like variations of the same picture (差分), and
	// removes the individual pages.
	MergeVariations        bool     `json:"merge_variations"`
	VariationMinSimilarity float64  `json:"variation_min_similarity"`
	VariationDelayMS       int      `json:"variation_delay_ms"`
	BlacklistTags          []string `json:"blacklist_tags"`
	APIRequestsPerSecond   float64  `json:"api_requests_per_second"`
	Sources                []Source `json:"sources"`
}

// Default returns the built-in default configuration.
func Default() *Config {
	return &Config{
		ImagesDir:              DefaultImagesDir(),
		StateDir:               DefaultStateDir(),
		Concurrency:            4,
		MaxRetries:             3,
		MaxPageCount:           10,
		SkipAI:                 true,
		SkipR18:                true,
		SkipR18G:               true,
		SkipQR:                 true,
		SkipMosaic:             true,
		SkipBlur:               true,
		QRMinCoverage:          0.02,
		MergeVariations:        true,
		VariationMinSimilarity: 0.9,
		VariationDelayMS:       1000,
		BlacklistTags:          []string{"漫画", "講座", "描き方", "MMD", "3D", "3Dカスタム少女"},
		APIRequestsPerSecond:   2,
		Sources: []Source{
			{Type: "bookmarks", Restrict: "public", Limit: 100},
			{Type: "bookmarks", Restrict: "private", Limit: 100},
			{Type: "recommended", Limit: 100},
		},
	}
}

// Load reads the configuration file; an empty path returns the defaults.
// When explicit is true, a missing file is an error.
func Load(path string, explicit bool) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return cfg, nil
}

// ResolvePath resolves the config path and reports whether it was set explicitly.
func ResolvePath(flagValue string) (path string, explicit bool) {
	if flagValue != "" {
		return flagValue, true
	}
	if v := os.Getenv("RANDAMU_CONFIG"); v != "" {
		return v, true
	}
	return filepath.Join(xdgConfigHome(), "randamu", "config.json"), false
}

// ApplyEnv overrides configuration with RANDAMU_* environment variables.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("RANDAMU_IMAGES_DIR"); v != "" {
		c.ImagesDir = v
	}
	if v := os.Getenv("RANDAMU_STATE_DIR"); v != "" {
		c.StateDir = v
	}
	if v := os.Getenv("RANDAMU_PROXY"); v != "" {
		c.Proxy = v
	}
}

// Validate checks the configuration and fills in defaults for missing fields.
func (c *Config) Validate() error {
	var err error
	if c.ImagesDir, err = expandHome(c.ImagesDir); err != nil {
		return err
	}
	if c.StateDir, err = expandHome(c.StateDir); err != nil {
		return err
	}
	if c.ImagesDir == "" {
		return fmt.Errorf("images_dir must not be empty")
	}
	if c.StateDir == "" {
		return fmt.Errorf("state_dir must not be empty")
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("concurrency must be greater than 0")
	}
	if c.MaxRetries < 1 {
		return fmt.Errorf("max_retries must be greater than 0")
	}
	if c.APIRequestsPerSecond <= 0 {
		c.APIRequestsPerSecond = 2
	}
	if c.VariationMinSimilarity <= 0 || c.VariationMinSimilarity > 1 {
		c.VariationMinSimilarity = 0.9
	}
	if c.QRMinCoverage <= 0 || c.QRMinCoverage > 1 {
		c.QRMinCoverage = 0.02
	}
	if c.VariationDelayMS <= 0 {
		c.VariationDelayMS = 1000
	}
	for i := range c.Sources {
		s := &c.Sources[i]
		if s.Limit <= 0 {
			s.Limit = 100
		}
		switch s.Type {
		case "bookmarks":
			if s.Restrict == "" {
				s.Restrict = "public"
			}
			if s.Restrict != "public" && s.Restrict != "private" {
				return fmt.Errorf("sources[%d]: restrict must be public or private", i)
			}
		case "search":
			if s.Tag == "" {
				return fmt.Errorf("sources[%d]: search source requires a tag", i)
			}
			if s.SearchTarget == "" {
				s.SearchTarget = "exact_match_for_tags"
			}
			if s.Sort == "" {
				s.Sort = "popular_desc"
			}
		case "recommended":
		default:
			return fmt.Errorf("sources[%d]: unknown source type %q", i, s.Type)
		}
	}
	return nil
}

// DefaultImagesDir returns the default image directory.
func DefaultImagesDir() string {
	return filepath.Join(xdgDataHome(), "randamu", "images")
}

// DefaultStateDir returns the default state directory.
func DefaultStateDir() string {
	return filepath.Join(xdgStateHome(), "randamu")
}

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".local", "share")
}

func xdgStateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), ".config")
}

func homeDir() string {
	if v := os.Getenv("HOME"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// expandHome expands a leading ~ in a path.
func expandHome(p string) (string, error) {
	if p == "~" {
		return homeDir(), nil
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:]), nil
	}
	return p, nil
}
