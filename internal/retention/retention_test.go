package retention

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// makeFile creates a file of the given size with the given age.
func makeFile(t *testing.T, dir, name string, size int64, age time.Duration) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := time.Now().Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestPruneAge(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "100_p0.jpg", 10, 72*time.Hour) // 3 天前
	makeFile(t, dir, "200.png", 10, 36*time.Hour)    // 1.5 天前
	makeFile(t, dir, "300_p1.jpg", 10, time.Hour)    // 新
	makeFile(t, dir, "keep.txt", 10, 720*time.Hour)  // 非本程序文件名

	deleted, err := Prune(dir, Options{MaxAgeDays: 2}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(deleted)
	want := []string{"100_p0.jpg"}
	if len(deleted) != 1 || deleted[0] != want[0] {
		t.Fatalf("deleted = %v, want %v", deleted, want)
	}
	if exists(dir, "100_p0.jpg") {
		t.Error("old page was not removed")
	}
	if !exists(dir, "200.png") || !exists(dir, "300_p1.jpg") {
		t.Error("files within the age limit were removed")
	}
	if !exists(dir, "keep.txt") {
		t.Error("unrecognised file was removed")
	}
}

func TestPruneSize(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "1_p0.jpg", 1<<20, 48*time.Hour)
	makeFile(t, dir, "2_p0.jpg", 1<<20, 24*time.Hour)
	makeFile(t, dir, "3_p0.jpg", 1<<20, time.Hour)
	makeFile(t, dir, "note.txt", 1<<20, 48*time.Hour) // 不参与统计也不删除

	deleted, err := Prune(dir, Options{MaxSizeMB: 2}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "1_p0.jpg" {
		t.Fatalf("deleted = %v, want [1_p0.jpg]", deleted)
	}
	if exists(dir, "1_p0.jpg") {
		t.Error("oldest file was not removed")
	}
	if !exists(dir, "2_p0.jpg") || !exists(dir, "3_p0.jpg") {
		t.Error("files within the size budget were removed")
	}
	if !exists(dir, "note.txt") {
		t.Error("unrecognised file was removed")
	}
}

func TestPruneSizeDeletesUntilUnderLimit(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"1_p0.jpg", "2_p0.jpg", "3_p0.jpg", "4_p0.jpg"} {
		makeFile(t, dir, name, 1<<20, time.Duration(4-i)*time.Hour)
	}

	deleted, err := Prune(dir, Options{MaxSizeMB: 1}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 3 {
		t.Fatalf("deleted = %v, want 3 files", deleted)
	}
	if !exists(dir, "4_p0.jpg") {
		t.Error("newest file was removed")
	}
}

func TestPruneMergedFiles(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "42.png", 10, 72*time.Hour)
	makeFile(t, dir, "42_v1.png", 10, 72*time.Hour)
	makeFile(t, dir, "42_p0.jpg", 10, 72*time.Hour)

	deleted, err := Prune(dir, Options{MaxAgeDays: 1}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 3 {
		t.Fatalf("deleted = %v, want 3 files", deleted)
	}
}

func TestPruneOrphanPart(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "1_p0.jpg.part", 10, 48*time.Hour) // 孤儿
	makeFile(t, dir, "2_p0.jpg.part", 10, time.Hour)    // 正在续传，保留
	makeFile(t, dir, "3_p0.jpg", 10, 48*time.Hour)      // 正常文件，不受 .part 规则影响

	deleted, err := Prune(dir, Options{MaxSizeMB: 100}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "1_p0.jpg.part" {
		t.Fatalf("deleted = %v, want [1_p0.jpg.part]", deleted)
	}
	if exists(dir, "1_p0.jpg.part") {
		t.Error("orphan .part was not removed")
	}
	if !exists(dir, "2_p0.jpg.part") || !exists(dir, "3_p0.jpg") {
		t.Error("files that must be kept were removed")
	}
}

func TestPruneDryRun(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "1_p0.jpg", 10, 72*time.Hour)

	deleted, err := Prune(dir, Options{MaxAgeDays: 1}, time.Now(), true, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "1_p0.jpg" {
		t.Fatalf("deleted = %v, want [1_p0.jpg]", deleted)
	}
	if !exists(dir, "1_p0.jpg") {
		t.Error("dry run removed a file")
	}
}

func TestPruneDisabled(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "1_p0.jpg", 10, 72*time.Hour)

	deleted, err := Prune(dir, Options{}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted = %v, want none", deleted)
	}
	if !exists(dir, "1_p0.jpg") {
		t.Error("disabled retention removed a file")
	}
}

func TestPruneMissingDir(t *testing.T) {
	deleted, err := Prune(filepath.Join(t.TempDir(), "nope"), Options{MaxSizeMB: 1}, time.Now(), false, logger())
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted = %v, want none", deleted)
	}
}
