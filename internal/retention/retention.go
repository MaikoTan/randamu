// Package retention enforces size and age limits on the image directory,
// deleting the oldest downloaded files first.
package retention

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Options selects the enforced limits; a zero value disables a limit.
type Options struct {
	// MaxSizeMB is the directory budget in MiB. When the downloaded files
	// take up more, the oldest ones are deleted first. 0 disables the check.
	MaxSizeMB int64
	// MaxAgeDays deletes downloaded files whose modification time is older
	// than this many days. 0 disables the check.
	MaxAgeDays int
}

// partMaxAge is how long the .part file of an interrupted download is kept
// before it is removed as an orphan.
const partMaxAge = 24 * time.Hour

var (
	// pageFile matches a downloaded page, optionally still a .part file.
	pageFile = regexp.MustCompile(`^\d+_p\d+\.(jpe?g|png|gif|webp)(\.part)?$`)
	// mergedFile matches a variation-merged APNG.
	mergedFile = regexp.MustCompile(`^\d+(_v\d+)?\.png$`)
)

// Prune deletes files that violate the limits, oldest first, and returns
// their names. In dry-run mode nothing is removed and the would-be deletions
// are returned. Only files randamu itself created (recognised by their file
// name) are touched; anything else in the directory is left alone.
func Prune(dir string, opts Options, now time.Time, dryRun bool, log *slog.Logger) ([]string, error) {
	if opts.MaxSizeMB <= 0 && opts.MaxAgeDays <= 0 {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type entry struct {
		name  string
		size  int64
		mtime time.Time
		gone  bool
	}
	var files []entry
	var orphans []entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // 争用中被删除,跳过
		}
		name := e.Name()
		if strings.HasSuffix(name, ".part") && pageFile.MatchString(name) {
			if now.Sub(info.ModTime()) > partMaxAge {
				orphans = append(orphans, entry{name: name, size: info.Size(), mtime: info.ModTime()})
			}
			continue
		}
		if !pageFile.MatchString(name) && !mergedFile.MatchString(name) {
			continue
		}
		files = append(files, entry{name: name, size: info.Size(), mtime: info.ModTime()})
	}

	// 最旧的优先删除
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })

	var deleted []string
	remove := func(f *entry) {
		f.gone = true
		deleted = append(deleted, f.name)
		if dryRun {
			log.Info("would delete file", "file", f.name)
			return
		}
		log.Debug("retention: removing file", "file", f.name)
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil && !os.IsNotExist(err) {
			log.Warn("retention: cannot remove file", "file", f.name, "error", err)
		}
	}

	// 中断下载遗留的 .part 文件
	for i := range orphans {
		remove(&orphans[i])
	}

	if opts.MaxAgeDays > 0 {
		cutoff := now.Add(-time.Duration(opts.MaxAgeDays) * 24 * time.Hour)
		for i := range files {
			if files[i].mtime.Before(cutoff) {
				remove(&files[i])
			}
		}
	}

	if opts.MaxSizeMB > 0 {
		limit := opts.MaxSizeMB << 20
		var total int64
		for _, f := range files {
			if !f.gone {
				total += f.size
			}
		}
		for i := range files {
			if total <= limit {
				break
			}
			if files[i].gone {
				continue
			}
			total -= files[i].size
			remove(&files[i])
		}
	}

	return deleted, nil
}
