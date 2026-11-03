// Package state persists cursors and dedup state and enforces a single instance.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const fileName = "state.json"

// State is persisted across runs.
type State struct {
	Version int               `json:"version"`
	Cursors map[string]string `json:"cursors"`
	Offsets map[string]int    `json:"offsets"`
	Viewed  []int64           `json:"viewed"`
	// Rejected illust ids whose images failed content analysis; they are
	// not re-downloaded and their pages are treated as completed.
	Rejected []int64   `json:"rejected,omitempty"`
	LastRun  time.Time `json:"last_run"`
}

// New returns an empty state.
func New() *State {
	return &State{
		Version: 1,
		Cursors: make(map[string]string),
		Offsets: make(map[string]int),
	}
}

// Load reads the state. A missing file returns empty state; a corrupt file
// returns empty state together with an error.
func Load(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(), nil
		}
		return New(), err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return New(), fmt.Errorf("corrupt state file: %w", err)
	}
	if s.Cursors == nil {
		s.Cursors = make(map[string]string)
	}
	if s.Offsets == nil {
		s.Offsets = make(map[string]int)
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return &s, nil
}

// Save writes the state atomically.
func (s *State) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.LastRun = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, fileName))
}

// AddViewed records a viewed work id, keeping only the most recent limit entries.
func (s *State) AddViewed(id int64, limit int) {
	for i, v := range s.Viewed {
		if v == id {
			s.Viewed = append(s.Viewed[:i], s.Viewed[i+1:]...)
			break
		}
	}
	s.Viewed = append(s.Viewed, id)
	if limit > 0 && len(s.Viewed) > limit {
		s.Viewed = s.Viewed[len(s.Viewed)-limit:]
	}
}

// RecentViewed returns at most n of the most recently viewed ids.
func (s *State) RecentViewed(n int) []int64 {
	if n <= 0 || len(s.Viewed) <= n {
		return s.Viewed
	}
	return s.Viewed[len(s.Viewed)-n:]
}

// AddRejected records a rejected illust id, keeping only the most recent
// limit entries so the list stays small.
func (s *State) AddRejected(id int64, limit int) {
	for i, v := range s.Rejected {
		if v == id {
			s.Rejected = append(s.Rejected[:i], s.Rejected[i+1:]...)
			break
		}
	}
	s.Rejected = append(s.Rejected, id)
	if limit > 0 && len(s.Rejected) > limit {
		s.Rejected = s.Rejected[len(s.Rejected)-limit:]
	}
}

// IsRejected reports whether the illust id was previously rejected.
func (s *State) IsRejected(id int64) bool {
	for _, v := range s.Rejected {
		if v == id {
			return true
		}
	}
	return false
}

// Lock acquires a single-instance lock and returns a release function.
func Lock(dir string) (release func(), err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, "lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			pid, _ := os.ReadFile(lockPath)
			return nil, fmt.Errorf("another instance is running (PID %s)", strconv.Quote(string(pid)))
		}
		return nil, err
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Close()

	return func() { _ = os.Remove(lockPath) }, nil
}
