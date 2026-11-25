package loci

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// State is kept per worktree in .git/loci/state.json, out of the working tree.
// Branch is the worktree's: every session in it shares one HEAD. What a session
// has visited is its own, so sessions that take turns in one worktree do not
// reset each other.
type State struct {
	Branch   string              `json:"branch"`
	Sessions map[string]*Session `json:"sessions,omitempty"`
	Pending  *Pending            `json:"pending,omitempty"`
}

// Session is what Loci remembers of one agent session in the worktree.
type Session struct {
	Visited []string  `json:"visited,omitempty"` // branches this session has left, oldest first
	Seen    time.Time `json:"seen"`              // its last command, to within seenEvery
}

// Pending records a boundary whose handoff the next session should receive.
type Pending struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Context int       `json:"context"`
	At      time.Time `json:"at"`
	Session string    `json:"session,omitempty"` // the session that crossed the boundary
}

// legacyState is the state.json of Loci before sessions were kept apart.
type legacyState struct {
	Session string   `json:"session"`
	Visited []string `json:"visited"`
}

const (
	maxSessions = 16               // sessions remembered per worktree, the most recent kept
	seenEvery   = time.Minute      // how stale Session.Seen may get before it is written
	liveFor     = 30 * time.Minute // a session seen this recently may still /clear
)

// session returns the entry for id, creating it, and drops the sessions
// that are too many or older than a pending handoff could live.
func (s *State) session(id string, at time.Time) *Session {
	if s.Sessions == nil {
		s.Sessions = map[string]*Session{}
	}
	if e := s.Sessions[id]; e != nil {
		return e
	}
	for k, e := range s.Sessions {
		if at.Sub(e.Seen) > pendingTTL {
			delete(s.Sessions, k)
		}
	}
	for len(s.Sessions) >= maxSessions {
		oldest := ""
		for k, e := range s.Sessions {
			if oldest == "" || e.Seen.Before(s.Sessions[oldest].Seen) {
				oldest = k
			}
		}
		delete(s.Sessions, oldest)
	}
	e := &Session{}
	s.Sessions[id] = e
	return e
}

// live reports whether session id ran a command within liveFor of at.
func (s *State) live(id string, at time.Time) bool {
	e := s.Sessions[id]
	return id != "" && e != nil && at.Sub(e.Seen) < liveFor
}

// maxVisited bounds the remembered branches; older ones count as new again.
const maxVisited = 50

// pendingTTL drops a handoff nobody picked up; by then it describes old work.
const pendingTTL = 24 * time.Hour

// remember appends br to the visited list once, keeping it most recent last.
func remember(visited []string, br string) []string {
	out := make([]string, 0, len(visited)+1)
	for _, v := range visited {
		if v != br {
			out = append(out, v)
		}
	}
	out = append(out, br)
	if len(out) > maxVisited {
		out = out[len(out)-maxVisited:]
	}
	return out
}

func statePath(gd string) string { return filepath.Join(gd, "loci", "state.json") }

// loadState reads the worktree's state; a missing or corrupt file is an empty state.
func loadState(gd string) (State, error) {
	var s State
	b, err := os.ReadFile(statePath(gd))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, nil
	}
	var old legacyState
	if json.Unmarshal(b, &old) == nil && old.Session != "" && s.Sessions == nil {
		s.Sessions = map[string]*Session{old.Session: {Visited: old.Visited}}
	}
	return s, nil
}

// saveState writes the state through a temporary file, so a reader never sees half of it.
func saveState(gd string, s State) error {
	p := statePath(gd)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
