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
type State struct {
	Branch  string   `json:"branch"`
	Session string   `json:"session,omitempty"` // the agent session Visited belongs to
	Visited []string `json:"visited,omitempty"` // branches this session has left, oldest first
	Pending *Pending `json:"pending,omitempty"`
}

// Pending records a boundary whose handoff the next session should receive.
type Pending struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Context int       `json:"context"`
	At      time.Time `json:"at"`
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
