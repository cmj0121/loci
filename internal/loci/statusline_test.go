package loci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// statusRepo is a repository with Loci's hooks installed, a handoff of 297k
// context in its log, and an empty home.
func statusRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := newRepo(t)
	if _, _, err := Install(dir, "/usr/local/bin/loci", false); err != nil {
		t.Fatal(err)
	}
	writeLog(t, filepath.Join(dir, ".git"),
		`{"level":"info","context":297000,"chars":400,"time":"2026-11-20T10:01:00+08:00","message":"handoff"}`)
	return dir
}

func statusline(t *testing.T, dir string, args ...string) string {
	t.Helper()
	code, out, errOut := run(t, `{"cwd":"`+dir+`"}`, append([]string{"statusline"}, args...)...)
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	return out
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatuslineWrapsThen(t *testing.T) {
	dir := statusRepo(t)
	if out := statusline(t, dir, "--then", "echo base"); out != "base · loci working · 297k saved\n" {
		t.Errorf("got %q", out)
	}
	if out := statusline(t, dir, "--then", "head -c 7"); out != `{"cwd": · loci working · 297k saved`+"\n" {
		t.Errorf("stdin not passed on: got %q", out)
	}
	real, _ := filepath.EvalSymlinks(dir)
	code, out, _ := run(t, `{"cwd":"/nonexistent","workspace":{"current_dir":"`+dir+`"}}`, "statusline", "--then", "pwd -P")
	if code != 0 || out != real+" · loci working · 297k saved\n" {
		t.Errorf("workspace.current_dir: exit %d got %q", code, out)
	}
}

func TestStatuslineRunsUserStatusLine(t *testing.T) {
	dir := statusRepo(t)
	writeJSON(t, filepath.Join(dir, ".claude", "settings.json"),
		map[string]any{"statusLine": map[string]any{"type": "command", "command": "'/opt/my loci/loci' statusline"}})
	writeJSON(t, filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"),
		map[string]any{"statusLine": map[string]any{"type": "command", "command": "echo user"}})
	if out := statusline(t, dir); out != "user · loci working · 297k saved\n" {
		t.Errorf("got %q", out)
	}
	writeJSON(t, filepath.Join(dir, ".claude", "settings.json"),
		map[string]any{"statusLine": map[string]any{"type": "command", "command": "echo project"}})
	if out := statusline(t, dir); out != "project · loci working · 297k saved\n" {
		t.Errorf("project status line: got %q", out)
	}
}

func TestStatuslineWithoutHooks(t *testing.T) {
	dir := statusRepo(t)
	if _, _, err := Install(dir, "/usr/local/bin/loci", true); err != nil {
		t.Fatal(err)
	}
	if out := statusline(t, dir, "--then", "echo base"); out != "base\n" {
		t.Errorf("got %q", out)
	}
}

func TestStatuslineNeverFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	isolateGit(t)
	dir := t.TempDir()
	if out := statusline(t, dir, "--then", "echo partial; exit 3"); out != "\n" {
		t.Errorf("failing --then: got %q", out)
	}
	if out := statusline(t, dir, "--then", "echo base"); out != "base\n" {
		t.Errorf("non-repo: got %q", out)
	}
	t.Chdir(dir)
	if code, out, _ := run(t, "not json", "statusline"); code != 0 || out != "\n" {
		t.Errorf("bad stdin: exit %d got %q", code, out)
	}
}

func TestStatuslineReadsCache(t *testing.T) {
	dir := statusRepo(t)
	statusline(t, dir)
	p := filepath.Join(dir, ".git", "loci", "status.json")
	var c map[string]any
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &c) != nil || c["key"] == "" || c["cleared"] != 297000.0 {
		t.Fatalf("cache %s: %v", b, err)
	}
	c["cleared"] = 1_200_000
	writeJSON(t, p, c)
	if out := statusline(t, dir); out != "loci working · 1.2M saved\n" {
		t.Errorf("unchanged logs: got %q", out)
	}
}

func TestStatuslineClearReady(t *testing.T) {
	dir := statusRepo(t)
	gd := filepath.Join(dir, ".git")
	if err := saveState(gd, State{Branch: "feat/x", Pending: &Pending{From: "main", To: "feat/x", At: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if out := statusline(t, dir); out != "loci working · 297k saved · /clear ready\n" {
		t.Errorf("pending: got %q", out)
	}
	if err := saveState(gd, State{Branch: "feat/x"}); err != nil {
		t.Fatal(err)
	}
	if out := statusline(t, dir); out != "loci working · 297k saved\n" {
		t.Errorf("cleared: got %q", out)
	}
}

func TestStatuslineRecentFailures(t *testing.T) {
	dir := statusRepo(t)
	at, _ := time.Parse(time.RFC3339, "2026-11-22T12:00:00+08:00")
	defer func(f func() time.Time) { now = f }(now)
	now = func() time.Time { return at }
	writeLog(t, filepath.Join(dir, ".git"),
		`{"level":"error","error":"old","time":"2026-11-21T11:00:00+08:00","message":"hook failed"}`,
		`{"level":"error","error":"new","time":"2026-11-22T11:00:00+08:00","message":"hook failed"}`)
	if out := statusline(t, dir); out != "loci working · 1 failed\n" {
		t.Errorf("got %q", out)
	}
	now = func() time.Time { return at.Add(24 * time.Hour) }
	if out := statusline(t, dir); out != "loci working\n" {
		t.Errorf("a day later: got %q", out)
	}
}

func TestStatuslineStalePendingNotReady(t *testing.T) {
	dir := statusRepo(t)
	at, _ := time.Parse(time.RFC3339, "2026-11-22T12:00:00+08:00")
	defer func(f func() time.Time) { now = f }(now)
	now = func() time.Time { return at }
	if err := saveState(filepath.Join(dir, ".git"), State{Branch: "feat/x", Pending: &Pending{From: "main", To: "feat/x", At: at.Add(-time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if out := statusline(t, dir); out != "loci working · 297k saved · /clear ready\n" {
		t.Errorf("fresh: got %q", out)
	}
	now = func() time.Time { return at.Add(pendingTTL) }
	if out := statusline(t, dir); out != "loci working · 297k saved\n" {
		t.Errorf("older than 24h: got %q", out)
	}
}
