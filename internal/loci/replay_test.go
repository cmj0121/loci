package loci

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tx builds transcript lines.
type tx struct {
	lines []string
	cwd   string
	clock int
}

func (x *tx) ts() string {
	x.clock++
	return fmt.Sprintf("2026-11-20T10:%02d:00Z", x.clock)
}

// turn adds a model call; bash, when set, is a shell command it starts.
func (x *tx) turn(id, branch string, in, write, read, out int, bash string) {
	content := `[]`
	if bash != "" {
		content = fmt.Sprintf(`[{"type":"tool_use","id":%q,"name":"Bash"}]`, bash)
	}
	x.lines = append(x.lines, fmt.Sprintf(`{"type":"assistant","cwd":%q,"gitBranch":%q,"timestamp":%q,"message":{"id":%q,"content":%s,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}}}`,
		x.cwd, branch, x.ts(), id, content, in, write, read, out))
}

// result finishes shell command bash; branch is what git reports after it.
func (x *tx) result(bash, branch string) {
	x.lines = append(x.lines, fmt.Sprintf(`{"type":"user","cwd":%q,"gitBranch":%q,"timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q}]}}`,
		x.cwd, branch, x.ts(), bash))
}

func (x *tx) write(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".jsonl"), []byte(strings.Join(x.lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestReplayModelsAHandoff(t *testing.T) {
	repo, proj := "/work/repo", t.TempDir()
	x := &tx{cwd: repo}
	x.turn("a1", "feat/a", 10, 1000, 0, 100, "")     // context 1,010: the session's floor
	x.turn("a2", "feat/a", 10, 0, 150_000, 10, "b1") // 150,010
	x.result("b1", "feat/a")                         // first sight
	x.turn("a3", "feat/a", 10, 0, 200_000, 10, "b2") // 200,010
	x.turn("a3", "feat/a", 10, 0, 200_000, 10, "")   // the same message again: counted once
	x.lines = append(x.lines, `{"type":"assistant","isSidechain":true,"message":{"id":"s","usage":{"input_tokens":99}}}`)
	x.result("b2", "feat/b")                       // a new branch at 200k: suggest
	x.turn("a4", "feat/b", 10, 0, 210_000, 10, "") // modelled as 1,310 + 10,000 grown
	x.write(t, proj, "s1")
	other := &tx{cwd: "/work/other"}
	other.turn("o1", "feat/z", 10, 0, 500_000, 10, "")
	other.write(t, proj, "s2") // another repository: left out

	res, err := Replay(ReplayOptions{Projects: []string{proj}, Repo: repo, Config: defaultConfig(), HandoffTokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 1 || res.Turns != 4 || res.Switches != 1 || len(res.Fires) != 1 {
		t.Fatalf("result %+v", res)
	}
	if f := res.Fires[0]; f.From != "feat/a" || f.To != "feat/b" || f.Context != 200_010 || f.Session != "s1" {
		t.Errorf("fire %+v", f)
	}
	// Now: 1,010 + 150,010 + 200,010 + 210,010. Under Loci the last turn reads
	// 11,300 from the cache and writes the new 1,310-token prefix.
	want := Spend{
		Tokens:     561_040,
		LociTokens: 1010 + 150_010 + 200_010 + (10 + 1310 + 11_300),
		Cost:       1760 + 15_060 + 20_060 + 21_060,
		LociCost:   1760 + 15_060 + 20_060 + (10 + 1310*1.25 + 11_300*0.1 + 50),
	}
	got := res.Total
	if !near(got.Tokens, want.Tokens) || !near(got.LociTokens, want.LociTokens) || !near(got.Cost, want.Cost) || !near(got.LociCost, want.LociCost) {
		t.Errorf("total %+v, want %+v", got, want)
	}
	if res.ByPurpose["feature"] == nil || len(res.ByPurpose) != 1 {
		t.Errorf("by purpose %v", res.ByPurpose)
	}
}

func TestReplayFollowsTheRule(t *testing.T) {
	repo, proj := "/work/repo", t.TempDir()
	x := &tx{cwd: repo}
	x.turn("a1", "feat/a", 10, 0, 1000, 10, "b1")
	x.result("b1", "feat/a")
	x.turn("a2", "feat/a", 10, 0, 300_000, 10, "b2")
	x.result("b2", "main") // long-lived
	x.turn("a3", "main", 10, 0, 300_000, 10, "b3")
	x.result("b3", "feat/a") // revisit
	x.turn("a4", "feat/a", 10, 0, 300_000, 10, "b4")
	x.result("b4", "HEAD") // detached: ignored
	x.turn("a5", "feat/a", 10, 0, 50_000, 10, "b5")
	x.result("b5", "fix/b") // small
	x.write(t, proj, "s1")
	// a later session in an agent worktree keeps its own state
	y := &tx{cwd: repo + "/.claude/worktrees/w1/src"}
	y.turn("c1", "feat/w", 10, 0, 1000, 10, "d1")
	y.result("d1", "feat/w") // first sight in this worktree
	y.clock = 30
	y.write(t, proj, "s2")
	res, err := Replay(ReplayOptions{Projects: []string{proj}, Repo: repo, Config: defaultConfig(), HandoffTokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 2 || res.Switches != 3 || len(res.Fires) != 0 {
		t.Fatalf("result %+v", res)
	}
	for _, r := range []string{ReasonLongLived, ReasonRevisit, ReasonSmall} {
		if res.Reasons[r] != 1 {
			t.Errorf("reasons %v", res.Reasons)
		}
	}
	if s := res.Total; s.Tokens != s.LociTokens || s.Cost != s.LociCost {
		t.Errorf("no fire, yet a saving: %+v", s)
	}
	if !strings.Contains(res.Table(5), "Branch switches judged: 3 (suggest 0, revisit 1, long-lived 1, small 1)") {
		t.Errorf("table:\n%s", res.Table(5))
	}
}

func TestReplayStopsModellingAfterACompaction(t *testing.T) {
	repo, proj := "/work/repo", t.TempDir()
	x := &tx{cwd: repo}
	x.turn("a1", "feat/a", 10, 1000, 0, 10, "b1")
	x.result("b1", "feat/a")
	x.turn("a2", "feat/a", 10, 0, 300_000, 10, "b2")
	x.result("b2", "feat/b")                      // suggest at 300k
	x.turn("a3", "feat/b", 10, 0, 40_000, 10, "") // smaller than at the suggestion: it compacted
	x.write(t, proj, "s1")
	res, err := Replay(ReplayOptions{Projects: []string{proj}, Repo: repo, Config: defaultConfig(), HandoffTokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	// The compacted turn is taken as it was; only the handoff's cache write is added.
	if d := res.Total.LociTokens - res.Total.Tokens; d != 1010+300 {
		t.Errorf("difference %v, want the 1,310-token handoff prefix", d)
	}
}

func TestPurposeAndWorktree(t *testing.T) {
	for br, want := range map[string]string{"feat/x": "feature", "fix/y": "bugfix", "hotfix/z": "bugfix", "chore/w": "other", "": "other"} {
		if got := purpose(br); got != want {
			t.Errorf("purpose(%q) = %s", br, got)
		}
	}
	for cwd, want := range map[string]string{
		"/r":                        "/r",
		"/r/sub/dir":                "/r",
		"/r/.claude/worktrees/w1/x": "/r/.claude/worktrees/w1",
		"/elsewhere":                "/elsewhere",
		"/rr":                       "/rr",
	} {
		if got := worktreeOf("/r", cwd); got != want {
			t.Errorf("worktreeOf(%q) = %s, want %s", cwd, got, want)
		}
	}
}

func TestProjectDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"-work-my-repo", "-work-my-repo--claude-worktrees-w1", "-work-my-repo2", "-work-other"} {
		_ = os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	got := ProjectDirs(root, "/work/my.repo")
	if len(got) != 2 || filepath.Base(got[0]) != "-work-my-repo" || filepath.Base(got[1]) != "-work-my-repo--claude-worktrees-w1" {
		t.Errorf("ProjectDirs = %v", got)
	}
}

func TestReplayCommand(t *testing.T) {
	dir := newRepo(t)
	proj := t.TempDir()
	top := sh(t, dir, "git", "rev-parse", "--show-toplevel")
	x := &tx{cwd: top}
	x.turn("a1", "feat/a", 10, 1000, 0, 10, "b1")
	x.result("b1", "feat/a")
	x.turn("a2", "feat/a", 10, 0, 300_000, 10, "b2")
	x.result("b2", "feat/b")
	x.turn("a3", "feat/b", 10, 0, 310_000, 10, "")
	x.write(t, proj, "abcdef0123")
	code, out, errs := run(t, "", "replay", "-C", dir, "--projects", proj)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	for _, want := range []string{"Replayed 1 sessions, 3 model turns.", "300k  feat/a → feat/b  (abcdef01, ", "total"} {
		if !strings.Contains(out, want) {
			t.Errorf("table misses %q:\n%s", want, out)
		}
	}
	code, out, _ = run(t, "", "replay", "-C", dir, "--projects", proj, "--json", "--threshold", "400000")
	var m map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &m) != nil || m["threshold"] != 400000.0 || len(m["fires"].([]any)) != 0 {
		t.Errorf("exit %d json %v", code, m)
	}
	t.Setenv("HOME", t.TempDir())
	if code, _, errs = run(t, "", "replay", "-C", dir); code != 1 || !strings.Contains(errs, "no Claude Code sessions") {
		t.Errorf("no sessions: exit %d %q", code, errs)
	}
}
