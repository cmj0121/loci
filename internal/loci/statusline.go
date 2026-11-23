package loci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// statusTimeout bounds the wrapped status line command, which Claude Code waits on.
const statusTimeout = 2 * time.Second

// statusEnv marks a status line run, so a wrapped command that calls Loci again
// prints only the segment instead of recursing.
const statusEnv = "LOCI_STATUSLINE"

// failureWindow is how long a hook failure stays in the status line.
const failureWindow = 24 * time.Hour

type StatuslineCmd struct {
	Then string `help:"Status line command to run first (default: the project's, else the user's)."`
}

// Run prints the wrapped status line and Loci's segment on one line. It always
// exits 0: a broken status line must not break the agent's display.
func (c *StatuslineCmd) Run(e *env) error {
	in, _ := io.ReadAll(e.stdin)
	var p struct {
		Cwd       string `json:"cwd"`
		Workspace struct {
			CurrentDir string `json:"current_dir"`
			ProjectDir string `json:"project_dir"`
		} `json:"workspace"`
	}
	_ = json.Unmarshal(in, &p)
	dir := p.Workspace.CurrentDir
	if dir == "" {
		dir = p.Cwd
	}
	if dir == "" {
		dir = "."
	}
	cmd := c.Then
	switch {
	case os.Getenv(statusEnv) != "":
		cmd = ""
	case cmd == "" || oursStatus.MatchString(cmd):
		top, _ := git(dir, "rev-parse", "--show-toplevel")
		if pd := p.Workspace.ProjectDir; pd != "" {
			if ptop, _ := git(pd, "rev-parse", "--show-toplevel"); ptop != top {
				top = "" // the session's project is another repository: trust only the user's
			}
		}
		cmd = previousStatus(top)
	}
	var parts []string
	for _, s := range []string{runStatus(cmd, dir, in), statusSegment(dir)} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	_, _ = fmt.Fprintln(e.stdout, strings.Join(parts, " · "))
	return nil
}

// runStatus runs a status line command with the session JSON on stdin; "" on failure.
func runStatus(command, dir string, stdin []byte) string {
	if command == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = append(os.Environ(), statusEnv+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// On timeout kill the whole group, not just sh, so no grandchild outlives it.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond // a background child must not hold the pipe open
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}

// statusCache is the Stats summary the status line shows, valid while Key matches.
type statusCache struct {
	Key       string      `json:"key"`
	Cleared   int         `json:"cleared"`
	Delegated int         `json:"delegated"`
	PendingAt []time.Time `json:"pendingAt"`
	FailedAt  []time.Time `json:"failedAt"`
}

// statusSegment is Loci's part of the status line, or "" when its hooks are
// not installed in the repository at dir.
func statusSegment(dir string) string {
	out, err := git(dir, "rev-parse", "--show-toplevel", "--path-format=absolute", "--git-common-dir")
	ls := strings.Split(out, "\n")
	if err != nil || len(ls) != 2 || !installed(ls[0]) {
		return ""
	}
	common := ls[1]
	gitDirs, _ := filepath.Glob(filepath.Join(common, "worktrees", "*"))
	gitDirs = append([]string{common}, gitDirs...)
	key := cacheKey(gitDirs)
	at := now()
	path := filepath.Join(common, "loci", "status.json")
	var c statusCache
	if b, err := os.ReadFile(path); err != nil || json.Unmarshal(b, &c) != nil || c.Key != key {
		s, err := Gather(dir, 0, at)
		if err != nil {
			return ""
		}
		c = statusCache{Key: key, Cleared: s.Cleared, Delegated: s.Delegated, PendingAt: pendingSince(gitDirs), FailedAt: failuresSince(gitDirs, at.Add(-failureWindow))}
		if b, err := json.Marshal(c); err == nil {
			_ = writeAtomic(path, b)
		}
	}
	seg := "loci working"
	if c.Cleared > 0 || c.Delegated > 0 {
		// main/subagents: context the handoffs cleared / context subagents kept out
		seg += " · " + tokens(c.Cleared) + "/" + tokens(c.Delegated) + " saved"
	}
	if recent(c.PendingAt, at, pendingTTL) > 0 {
		hint := "new branch: /clear to hand off"
		if os.Getenv("NO_COLOR") == "" {
			hint = "\x1b[33m" + hint + "\x1b[0m" // yellow, unless no-color.org's NO_COLOR is set
		}
		seg += " · " + hint
	}
	if failed := recent(c.FailedAt, at, failureWindow); failed > 0 {
		seg += fmt.Sprintf(" · %d failed", failed)
	}
	return seg
}

// writeAtomic replaces path through a temporary file in its directory, so a
// symlink at path is replaced rather than followed.
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// recent counts the times no older than d at now, so a cached entry expires by itself.
func recent(ts []time.Time, now time.Time, d time.Duration) int {
	n := 0
	for _, t := range ts {
		if now.Sub(t) <= d {
			n++
		}
	}
	return n
}

// pendingSince lists when each worktree's pending suggestion was made.
func pendingSince(gitDirs []string) []time.Time {
	var out []time.Time
	for _, gd := range gitDirs {
		if st, err := loadState(gd); err == nil && st.Pending != nil {
			out = append(out, st.Pending.At)
		}
	}
	return out
}

// installed reports whether Loci's hooks are in the repository's local settings.
func installed(top string) bool {
	b, err := os.ReadFile(settingsPath(top))
	if err != nil {
		return false
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	for _, groups := range s.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if ours.MatchString(h.Command) {
					return true
				}
			}
		}
	}
	return false
}

// cacheKey changes whenever a worktree's log or state does.
func cacheKey(gitDirs []string) string {
	var b strings.Builder
	for _, gd := range gitDirs {
		for _, p := range []string{logPath(gd), statePath(gd)} {
			if fi, err := os.Stat(p); err == nil {
				fmt.Fprintf(&b, "%s:%d:%d;", p, fi.Size(), fi.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// failuresSince lists the times of the hook failures logged after cut.
func failuresSince(gitDirs []string, cut time.Time) []time.Time {
	var out []time.Time
	for _, gd := range gitDirs {
		b, err := os.ReadFile(logPath(gd))
		if err != nil {
			continue
		}
		for l := range strings.SplitSeq(string(b), "\n") {
			var e logEntry
			if json.Unmarshal([]byte(l), &e) == nil && e.Message == msgFailed && e.Time.After(cut) {
				out = append(out, e.Time)
			}
		}
	}
	return out
}
