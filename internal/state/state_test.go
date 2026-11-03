package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New()
	s.Cursors["bookmarks:public"] = "https://app-api.pixiv.net/v1/user/bookmarks/illust?x=1"
	s.Offsets["recommended"] = 30
	s.AddViewed(42, 500)

	if err := s.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Cursors["bookmarks:public"] != s.Cursors["bookmarks:public"] {
		t.Fatalf("cursor not persisted: %+v", loaded.Cursors)
	}
	if loaded.Offsets["recommended"] != 30 {
		t.Fatalf("offset not persisted: %+v", loaded.Offsets)
	}
	if len(loaded.Viewed) != 1 || loaded.Viewed[0] != 42 {
		t.Fatalf("viewed not persisted: %+v", loaded.Viewed)
	}
	if loaded.LastRun.IsZero() {
		t.Fatal("last_run not written")
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Cursors == nil || s.Offsets == nil {
		t.Fatal("empty state must initialize maps")
	}
}

func TestLoadCorruptReturnsEmptyWithError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err == nil {
		t.Fatal("a corrupt file must return an error")
	}
	if len(s.Cursors) != 0 {
		t.Fatal("a corrupt file must not yield partial state")
	}
}

func TestAddViewedDedupAndCap(t *testing.T) {
	s := New()
	s.AddViewed(1, 3)
	s.AddViewed(2, 3)
	s.AddViewed(3, 3)
	s.AddViewed(1, 3) // duplicate moves to the end
	if got := s.Viewed; len(got) != 3 || got[2] != 1 {
		t.Fatalf("dedup failed: %v", got)
	}
	s.AddViewed(4, 3)
	if got := s.Viewed; len(got) != 3 || got[0] != 3 || got[2] != 4 {
		t.Fatalf("cap failed: %v", got)
	}
}

func TestRecentViewed(t *testing.T) {
	s := New()
	for i := int64(1); i <= 5; i++ {
		s.AddViewed(i, 10)
	}
	if got := s.RecentViewed(2); len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("RecentViewed: %v", got)
	}
	if got := s.RecentViewed(0); len(got) != 5 {
		t.Fatalf("n<=0 must return everything: %v", got)
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	release, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if _, err := Lock(dir); err == nil {
		t.Fatal("the second lock acquisition must fail")
	}
	release()
	release2, err := Lock(dir)
	if err != nil {
		t.Fatalf("lock must succeed after release: %v", err)
	}
	release2()
}
