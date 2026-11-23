package loci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeLog(t *testing.T, gd string, ls ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(gd, "loci"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath(gd), []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGatherCountsEveryWorktree(t *testing.T) {
	dir := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	sh(t, dir, "git", "worktree", "add", "-q", "-b", "feat/wt", wt)
	writeLog(t, filepath.Join(dir, ".git"),
		`{"level":"info","reason":"suggest","context":300000,"suggested":true,"time":"2026-11-20T10:00:00+08:00","message":"boundary"}`,
		`{"level":"info","context":300000,"chars":400,"time":"2026-11-20T10:01:00+08:00","message":"handoff"}`,
		`{"level":"info","reason":"revisit","suggested":false,"time":"2026-11-20T11:00:00+08:00","message":"boundary"}`,
		`not json`,
		`{"level":"info","reason":"suggest","context":150000,"time":"2026-11-21T09:00:00+08:00","message":"boundary"}`,
		`{"level":"info","to":"feat/x","context":150000,"time":"2026-11-21T09:30:00+08:00","message":"replaced"}`,
	)
	gdWt := sh(t, wt, "git", "rev-parse", "--absolute-git-dir")
	writeLog(t, gdWt,
		`{"level":"info","reason":"suggest","context":200000,"time":"2026-11-22T09:00:00+08:00","message":"boundary"}`,
		`{"level":"info","context":200000,"chars":300,"time":"2026-11-22T09:05:00+08:00","message":"handoff"}`,
		`{"level":"error","error":"boom","time":"2026-11-22T09:06:00+08:00","message":"hook failed"}`,
	)
	if err := saveState(gdWt, State{Pending: &Pending{To: "feat/y", At: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	s, err := Gather(wt, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Logs != 2 || s.Switches != 4 || s.Suggested != 3 || s.Handoffs != 2 || s.Replaced != 1 || s.Waiting != 1 || s.Failures != 1 {
		t.Errorf("stats = %+v", s)
	}
	if s.Cleared != 500_000 || s.Reasons[ReasonRevisit] != 1 {
		t.Errorf("cleared %d reasons %v", s.Cleared, s.Reasons)
	}
	if a := s.Adoption(); a < 0.66 || a > 0.67 {
		t.Errorf("adoption %.3f", a)
	}
	tab := s.Table()
	for _, want := range []string{"Loci stats for repo (2026-11-20 to 2026-11-22)", "67% adopted", "500k", "250k on average"} {
		if !strings.Contains(tab, want) {
			t.Errorf("table misses %q:\n%s", want, tab)
		}
	}
	at, _ := time.Parse(time.RFC3339, "2026-11-22T12:00:00+08:00")
	s, _ = Gather(dir, 30*time.Hour, at)
	if s.Switches != 2 || s.Handoffs != 1 || s.Replaced != 1 || s.Cleared != 200_000 {
		t.Errorf("--since 30h = %+v", s)
	}
}

func TestStatsCommand(t *testing.T) {
	dir := newRepo(t)
	code, out, _ := run(t, "", "stats", "-C", dir)
	if code != 0 || !strings.Contains(out, "No log yet") {
		t.Errorf("exit %d:\n%s", code, out)
	}
	writeLog(t, filepath.Join(dir, ".git"), `{"reason":"suggest","time":"2026-11-20T10:00:00Z","message":"boundary"}`)
	code, out, _ = run(t, "", "stats", "-C", dir, "--json", "--since", "3650d")
	var m map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &m) != nil || m["suggested"] != 1.0 || m["adoption"] != 0.0 {
		t.Errorf("exit %d json %v", code, m)
	}
	if code, _, _ = run(t, "", "stats", "-C", dir, "--since", "week"); code != 2 {
		t.Errorf("--since week exit %d, want 2", code)
	}
}

func TestTokens(t *testing.T) {
	for n, want := range map[int]string{950: "950", 12_345: "12k", 1_234_567: "1.2M"} {
		if got := tokens(n); got != want {
			t.Errorf("tokens(%d) = %s", n, got)
		}
	}
}

func TestGatherCountsEachSubagentOnce(t *testing.T) {
	dir := newRepo(t)
	writeLog(t, filepath.Join(dir, ".git"),
		`{"level":"info","agent":"a1","context":30000,"time":"2026-11-20T10:00:00+08:00","message":"subagent"}`,
		// a1 was resumed and stopped again with more context: it counts once, at its largest.
		`{"level":"info","agent":"a1","context":45000,"time":"2026-11-20T10:05:00+08:00","message":"subagent"}`,
		`{"level":"info","agent":"a2","context":20000,"time":"2026-11-20T10:06:00+08:00","message":"subagent"}`,
		`{"level":"info","context":99000,"time":"2026-11-20T10:07:00+08:00","message":"subagent"}`,
	)
	s, err := Gather(dir, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Subagents != 2 || s.Delegated != 65_000 || s.Cleared != 0 {
		t.Errorf("stats = %+v", s)
	}
	if tab := s.Table(); !strings.Contains(tab, "65k  2 subagents") {
		t.Errorf("table:\n%s", tab)
	}
}
