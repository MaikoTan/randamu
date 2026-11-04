// Command randamu periodically and idempotently downloads pixiv images to a local directory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MaikoTan/randamu/internal/analyze"
	"github.com/MaikoTan/randamu/internal/config"
	"github.com/MaikoTan/randamu/internal/download"
	"github.com/MaikoTan/randamu/internal/filter"
	"github.com/MaikoTan/randamu/internal/pixiv"
	"github.com/MaikoTan/randamu/internal/promo"
	"github.com/MaikoTan/randamu/internal/state"
	"github.com/MaikoTan/randamu/internal/variation"
)

// version is injected at build time via ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("randamu", flag.ContinueOnError)
	var (
		configFlag  = fs.String("config", "", "path to the config file")
		imagesDir   = fs.String("images-dir", "", "image download directory")
		stateDir    = fs.String("state-dir", "", "state directory")
		proxyFlag   = fs.String("proxy", "", "HTTP proxy URL")
		concurrency = fs.Int("concurrency", 0, "number of concurrent downloads")
		tokenFile   = fs.String("refresh-token-file", "", "path to the refresh token file")
		dryRun      = fs.Bool("dry-run", false, "list what would be downloaded without writing files")
		logLevel    = fs.String("log-level", "info", "log level: debug|info|warn|error")
		showVersion = fs.Bool("version", false, "print the version")
	)
	var sourceFlags multiFlag
	fs.Var(&sourceFlags, "source", "only process this source type (repeatable)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version)
		return 0
	}

	logger := newLogger(*logLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath, explicit := config.ResolvePath(*configFlag)
	cfg, err := config.Load(cfgPath, explicit)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 2
	}
	cfg.ApplyEnv()
	if *imagesDir != "" {
		cfg.ImagesDir = *imagesDir
	}
	if *stateDir != "" {
		cfg.StateDir = *stateDir
	}
	if *proxyFlag != "" {
		cfg.Proxy = *proxyFlag
	}
	if *concurrency > 0 {
		cfg.Concurrency = *concurrency
	}
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid config", "error", err)
		return 2
	}

	token, err := pixiv.ResolveRefreshToken(*tokenFile, cfg.StateDir)
	if err != nil {
		logger.Error("failed to resolve refresh_token", "error", err)
		return 3
	}

	httpClient, err := newHTTPClient(cfg.Proxy)
	if err != nil {
		logger.Error("failed to create HTTP client", "error", err)
		return 2
	}
	logger.Info("HTTP client created", "proxy", cfg.Proxy)

	client, err := pixiv.New(pixiv.Options{
		HTTPClient:        httpClient,
		MaxRetries:        cfg.MaxRetries,
		RequestsPerSecond: cfg.APIRequestsPerSecond,
		TokenFile:         filepath.Join(cfg.StateDir, "refresh_token"),
		Logger:            logger,
	})
	if err != nil {
		logger.Error("failed to initialize pixiv client", "error", err)
		return 1
	}
	if err := client.Login(ctx, token); err != nil {
		logger.Error("failed to log in to pixiv", "error", err)
		return 3
	}

	st, err := state.Load(cfg.StateDir)
	if err != nil {
		logger.Warn("state file unusable, resetting", "error", err)
	}

	release, err := state.Lock(cfg.StateDir)
	if err != nil {
		logger.Error("failed to acquire instance lock", "error", err)
		return 1
	}
	defer release()

	downloadOpts := download.Options{
		Dir:         cfg.ImagesDir,
		MaxRetries:  cfg.MaxRetries,
		UserAgent:   pixiv.DefaultUserAgent,
		Referer:     pixiv.DefaultReferer,
		AllowedHost: "i.pximg.net",
		HTTPClient:  httpClient,
		Logger:      logger,
	}
	if cfg.SkipQR || cfg.SkipMosaic || cfg.SkipBlur || cfg.SkipPromoText {
		analyzeOpts := analyze.Options{
			QR:            cfg.SkipQR,
			Mosaic:        cfg.SkipMosaic,
			Blur:          cfg.SkipBlur,
			QRMinCoverage: cfg.QRMinCoverage,
		}
		downloadOpts.Analyze = func(path string) error {
			res := analyze.CheckFile(path, analyzeOpts)
			if res.DecodeErr != nil {
				// Analysis is best-effort: undecodable files (WebP) are kept.
				logger.Debug("content analysis skipped", "file", path, "error", res.DecodeErr)
				return nil
			}
			if res.Skip {
				return errors.New(res.Reason)
			}
			if cfg.SkipPromoText {
				kw, err := promo.Scan(path, promo.DefaultKeywords)
				switch {
				case errors.Is(err, promo.ErrUnavailable):
					logger.Debug("promo text check skipped: tesseract not in PATH", "file", path)
				case err != nil:
					logger.Debug("promo text check failed", "file", path, "error", err)
				case kw != "":
					return fmt.Errorf("promotional text mentions %q", kw)
				}
			}
			return nil
		}
	}
	downloader := download.New(downloadOpts)
	filterOpts := filter.Options{
		SkipAI:       cfg.SkipAI,
		SkipR18:      cfg.SkipR18,
		SkipR18G:     cfg.SkipR18G,
		MaxPageCount: cfg.MaxPageCount,
		Blacklist:    cfg.BlacklistTags,
	}

	sources := filterSources(cfg.Sources, sourceFlags)
	if len(sources) == 0 {
		logger.Error("no sources to process")
		return 2
	}

	userID := cfg.UserID
	if userID == 0 {
		userID = client.UserID()
	}

	failures := 0
	for _, src := range sources {
		if ctx.Err() != nil {
			break
		}
		n, err := runSource(ctx, client, st, cfg, src, userID, downloader, filterOpts, logger, *dryRun)
		failures += n
		if err != nil {
			logger.Error("source failed", "source", sourceKey(src), "error", err)
			failures++
		}
	}

	if !*dryRun {
		if err := st.Save(cfg.StateDir); err != nil {
			logger.Error("failed to save state", "error", err)
			return 1
		}
	}

	if ctx.Err() != nil {
		logger.Info("interrupted; cursors for unfinished pages were kept")
		return 0
	}
	if failures > 0 {
		logger.Warn("some jobs failed", "count", failures)
		return 4
	}
	logger.Info("done")
	return 0
}

// runSource processes one source until the per-run limit or the last page.
func runSource(
	ctx context.Context,
	client *pixiv.Client,
	st *state.State,
	cfg *config.Config,
	src config.Source,
	userID int64,
	d *download.Downloader,
	filterOpts filter.Options,
	logger *slog.Logger,
	dryRun bool,
) (int, error) {
	key := sourceKey(src)
	processed := 0
	failures := 0

	for processed < src.Limit {
		var page pixiv.Page
		var err error
		switch src.Type {
		case "bookmarks":
			page, err = client.BookmarksPage(ctx, userID, src.Restrict, st.Cursors[key])
		case "search":
			page, err = client.SearchPage(ctx, pixiv.SearchParams{
				Word:         src.Tag,
				SearchTarget: src.SearchTarget,
				Sort:         src.Sort,
				Duration:     src.Duration,
			}, st.Cursors[key])
		case "recommended":
			page, err = client.RecommendedPage(ctx, st.Offsets["recommended"], st.RecentViewed(200))
		default:
			return failures, fmt.Errorf("unknown source type %q", src.Type)
		}
		if err != nil {
			return failures, err
		}

		if len(page.Illusts) == 0 {
			// No more content: reset the cursor so the next run starts from the newest items.
			if src.Type == "recommended" {
				st.Offsets["recommended"] = 0
			} else {
				delete(st.Cursors, key)
			}
			break
		}

		var jobs []download.Job
		// pageNames tracks the files of each multi-page work so its variation
		// sets can be merged into APNGs after this page of jobs completes.
		pageNames := make(map[int64][]string)
		for _, illust := range page.Illusts {
			if processed >= src.Limit {
				break
			}
			processed++
			if src.Type == "recommended" {
				st.AddViewed(illust.ID, 500)
			}
			if skip, reason := filter.Skip(illust, filterOpts); skip {
				logger.Debug("skipping work", "id", illust.ID, "reason", reason, "source", key)
				continue
			}
			if st.IsRejected(illust.ID) {
				logger.Debug("skipping previously rejected work", "id", illust.ID, "source", key)
				continue
			}
			if cfg.MergeVariations {
				if mergedExists(cfg.ImagesDir, illust.ID) {
					logger.Debug("skipping merged work", "id", illust.ID, "source", key)
					continue
				}
			}
			var names []string
			complete := true
			for pageIdx, rawURL := range filter.OriginalURLs(illust) {
				name, err := download.FileName(illust.ID, pageIdx, rawURL)
				if err != nil {
					logger.Warn("cannot derive file name", "id", illust.ID, "error", err)
					complete = false
					continue
				}
				names = append(names, name)
				jobs = append(jobs, download.Job{
					URL:      rawURL,
					Filename: name,
					Source:   key,
					IllustID: illust.ID,
					Page:     pageIdx,
				})
			}
			if complete && len(names) > 1 {
				pageNames[illust.ID] = names
			}
		}

		pageFailures := runJobs(ctx, d, jobs, cfg.Concurrency, logger, dryRun, st)
		if cfg.MergeVariations && !dryRun {
			mergeVariations(cfg, pageNames, logger)
		}
		failures += pageFailures
		if pageFailures > 0 {
			// Page barrier: not every job on this page finished, so the cursor does not advance.
			if !dryRun {
				_ = st.Save(cfg.StateDir)
			}
			return failures, nil
		}

		switch src.Type {
		case "recommended":
			st.Offsets["recommended"] += len(page.Illusts)
		default:
			if page.NextURL != "" {
				st.Cursors[key] = page.NextURL
			} else {
				delete(st.Cursors, key)
			}
		}
		if !dryRun {
			if err := st.Save(cfg.StateDir); err != nil {
				return failures, err
			}
		}

		if src.Type != "recommended" && page.NextURL == "" {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	return failures, nil
}

// runJobs executes one page of download jobs concurrently and returns the
// failure count. Jobs rejected by content analysis are recorded in state and
// do not count as failures.
func runJobs(ctx context.Context, d *download.Downloader, jobs []download.Job, workers int, logger *slog.Logger, dryRun bool, st *state.State) int {
	if len(jobs) == 0 {
		return 0
	}
	if workers < 1 {
		workers = 1
	}

	ch := make(chan download.Job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range ch {
				if dryRun {
					logger.Info("would download", "file", job.Filename, "source", job.Source)
					continue
				}
				status, err := d.Download(ctx, job)
				switch {
				case status == download.Rejected:
					mu.Lock()
					st.AddRejected(job.IllustID, 10000)
					mu.Unlock()
					logger.Info("rejected by content analysis", "file", job.Filename, "illust", job.IllustID, "reason", err)
				case err != nil || status == download.Failed:
					mu.Lock()
					failures++
					mu.Unlock()
					logger.Warn("download failed", "file", job.Filename, "url", job.URL, "error", err)
				default:
					logger.Debug("download result", "file", job.Filename, "status", status.String())
				}
			}
		}()
	}

	func() {
		defer close(ch)
		for _, job := range jobs {
			select {
			case ch <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	return failures
}

// mergeVariations combines the downloaded pages of each work that look like
// variations of one picture (差分) into looping APNGs, then removes the pages
// it merged. Works whose pages failed, were rejected or do not resemble each
// other keep their page files untouched. A work may contain several variation
// sets and standalone pages; each set is merged on its own.
func mergeVariations(cfg *config.Config, works map[int64][]string, logger *slog.Logger) {
	delay := time.Duration(cfg.VariationDelayMS) * time.Millisecond
	for id, names := range works {
		if mergedExists(cfg.ImagesDir, id) {
			continue
		}
		paths := make([]string, 0, len(names))
		complete := true
		for _, name := range names {
			p := filepath.Join(cfg.ImagesDir, name)
			if _, err := os.Stat(p); err != nil {
				complete = false
				break
			}
			paths = append(paths, p)
		}
		if !complete {
			continue
		}
		opts := variation.Options{
			MinSimilarity:  cfg.VariationMinSimilarity,
			MatchCharacter: cfg.VariationCharacterMatch,
		}
		finals, err := variation.Merge(cfg.ImagesDir, id, paths, delay, opts)
		if err != nil {
			if errors.Is(err, variation.ErrNotVariation) {
				logger.Debug("pages are not variations; keeping them", "illust", id, "reason", err)
			} else {
				logger.Warn("cannot merge variations", "illust", id, "error", err)
			}
			continue
		}
		logger.Info("merged variations into APNG", "illust", id, "pages", len(paths), "files", finals)
	}
}

// mergedExists reports whether a previous run merged part of this work: the
// whole work into {id}.png, or, for a work with several variation sets, the
// first set into {id}_v1.png.
func mergedExists(dir string, id int64) bool {
	for _, name := range []string{fmt.Sprintf("%d.png", id), fmt.Sprintf("%d_v1.png", id)} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func filterSources(all []config.Source, want []string) []config.Source {
	if len(want) == 0 {
		return all
	}
	set := make(map[string]struct{}, len(want))
	for _, w := range want {
		set[w] = struct{}{}
	}
	var out []config.Source
	for _, s := range all {
		if _, ok := set[s.Type]; ok {
			out = append(out, s)
		}
	}
	return out
}

func sourceKey(s config.Source) string {
	switch s.Type {
	case "bookmarks":
		return "bookmarks:" + s.Restrict
	case "search":
		return strings.Join([]string{"search", s.Tag, s.SearchTarget, s.Sort, s.Duration}, ":")
	default:
		return "recommended"
	}
}

func newHTTPClient(proxy string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 16
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
