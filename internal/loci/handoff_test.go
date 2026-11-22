package loci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueOf(t *testing.T) {
	for br, want := range map[string]string{
		"feat/gh-7440-scan-queue": "gh-7440",
		"fix/123-typo":            "#123",
		"ABC-12-thing":            "ABC-12",
		"feat/no-issue":           "",
		"main":                    "",
	} {
		if got := issueOf(br); got != want {
			t.Errorf("issueOf(%q) = %q, want %q", br, got, want)
		}
	}
}

func TestCollectOnAFeatureBranch(t *testing.T) {
	dir := newRepo(t)
	sh(t, dir, "git", "switch", "-q", "-c", "fix/gh-8388-crawler")
	commitFile(t, dir, "a.go", "package a", "fix(gh-8388): add a")
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Collect(dir, "", "feat/gh-8380-old")
	if err != nil {
		t.Fatal(err)
	}
	if f.Repo != "repo" || f.Branch != "fix/gh-8388-crawler" || f.Issue != "gh-8388" || f.Previous != "feat/gh-8380-old" {
		t.Errorf("facts = %+v", f)
	}
	if f.Base != "main" || len(f.Commits) != 1 || !strings.HasSuffix(f.Commits[0], "fix(gh-8388): add a") {
		t.Errorf("base %q commits %v", f.Base, f.Commits)
	}
	if len(f.Status) != 1 || f.Status[0] != "?? b.go" {
		t.Errorf("status %v", f.Status)
	}
	md := f.Markdown()
	for _, want := range []string{"# Loci handoff", "- branch: fix/gh-8388-crawler", "- issue: gh-8388 (", "read it first",
		"- previous branch: feat/gh-8380-old", "- base: main", "- upstream: none", "## Commits since base (1)", "    ?? b.go"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown misses %q:\n%s", want, md)
		}
	}
}

func TestCollectPrefersOriginMainOverOriginHEAD(t *testing.T) {
	src := newRepo(t)
	sh(t, src, "git", "switch", "-q", "-c", "feat/x")
	clone := filepath.Join(t.TempDir(), "clone")
	sh(t, src, "git", "clone", "-q", src, clone) // origin/HEAD → feat/x
	if b := defaultBase(clone); b != "origin/main" {
		t.Errorf("defaultBase = %q, want origin/main", b)
	}
	sh(t, clone, "git", "config", "loci.base", "origin/feat/x")
	if b := defaultBase(clone); b != "origin/feat/x" {
		t.Errorf("loci.base ignored: %q", b)
	}
}

func TestCollectDropsAPreviousEqualToTheBranch(t *testing.T) {
	dir := newRepo(t)
	f, err := Collect(dir, "", "main")
	if err != nil {
		t.Fatal(err)
	}
	if f.Previous != "" || strings.Contains(f.Markdown(), "previous branch") {
		t.Errorf("previous = %q", f.Previous)
	}
}

func TestCollectCapsLongLists(t *testing.T) {
	dir := newRepo(t)
	sh(t, dir, "git", "switch", "-q", "-c", "feat/many")
	n := maxDiffStat + 5 // more files than the diffstat cap, more commits than the commit cap
	for i := range n {
		commitFile(t, dir, fmt.Sprintf("f%02d", i), "x", fmt.Sprintf("c%02d", i))
	}
	f, err := Collect(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Commits) != maxCommits || f.MoreCommits != n-maxCommits {
		t.Errorf("commits %d more %d", len(f.Commits), f.MoreCommits)
	}
	if len(f.DiffStat) != maxDiffStat || !strings.Contains(f.DiffStat[len(f.DiffStat)-1], "files changed") {
		t.Errorf("diffstat %d lines, last %q", len(f.DiffStat), f.DiffStat[len(f.DiffStat)-1])
	}
	if more := f.DiffStat[len(f.DiffStat)-2]; more != fmt.Sprintf("… %d more", n-maxDiffStat+2) {
		t.Errorf("diffstat says %q, want %d more (%d files)", more, n-maxDiffStat+2, n)
	}
	if !strings.Contains(f.Markdown(), fmt.Sprintf("… %d older", n-maxCommits)) {
		t.Error("markdown does not say how many commits were left out")
	}
}

func TestHandoffJSONHasArrays(t *testing.T) {
	dir := newRepo(t)
	code, out, errs := run(t, "", "handoff", "-C", dir, "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"commits", "diff_stat", "status"} {
		if _, ok := m[k].([]any); !ok {
			t.Errorf("%s = %v, want an array", k, m[k])
		}
	}
	if _, ok := m["previous"]; ok {
		t.Error("previous present on a fresh repository")
	}
}

func TestHandoffOutsideARepositoryFails(t *testing.T) {
	isolateGit(t)
	if code, _, _ := run(t, "", "handoff", "-C", t.TempDir()); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}
