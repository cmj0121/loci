package loci

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Caps keep a handoff under about 2k tokens (~4 characters per token).
const (
	maxCommits  = 20
	maxDiffStat = 30
	maxStatus   = 30
	maxChars    = 8000
)

// Facts is what git alone says about the work on the current branch.
type Facts struct {
	Repo        string   `json:"repo"`
	Branch      string   `json:"branch"`
	Issue       string   `json:"issue,omitempty"`
	Previous    string   `json:"previous,omitempty"`
	Base        string   `json:"base,omitempty"`
	MergeBase   string   `json:"merge_base,omitempty"`
	Upstream    string   `json:"upstream,omitempty"`
	Ahead       int      `json:"ahead"`
	Behind      int      `json:"behind"`
	Commits     []string `json:"commits"`
	MoreCommits int      `json:"more_commits,omitempty"`
	DiffStat    []string `json:"diff_stat"`
	Status      []string `json:"status"`
}

var (
	issueKey = regexp.MustCompile(`^([A-Za-z]+-\d+)`)
	issueNum = regexp.MustCompile(`^(\d+)\b`)
)

// issueOf takes an issue key from a branch name: feat/gh-7440-x → gh-7440, fix/123-x → #123.
func issueOf(branch string) string {
	seg := branch[strings.LastIndex(branch, "/")+1:]
	if m := issueKey.FindStringSubmatch(seg); m != nil {
		return m[1]
	}
	if m := issueNum.FindStringSubmatch(seg); m != nil {
		return "#" + m[1]
	}
	return ""
}

// Collect gathers Facts for the worktree at dir. base "" means defaultBase.
func Collect(dir, base, previous string) (Facts, error) {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Facts{}, err
	}
	// Empty lists, not nil, so the JSON form always carries arrays.
	f := Facts{Repo: filepath.Base(top), Commits: []string{}, DiffStat: []string{}, Status: []string{}}
	if f.Branch, err = currentBranch(dir); err != nil {
		return f, err
	}
	if f.Branch == "" {
		f.Branch = "(detached HEAD)"
	} else {
		f.Issue = issueOf(f.Branch)
	}
	if previous != f.Branch {
		f.Previous = previous
	}
	if base == "" {
		base = defaultBase(dir)
	}
	if base != "" {
		if mb, err := git(dir, "merge-base", base, "HEAD"); err == nil {
			f.Base, f.MergeBase = base, mb[:min(len(mb), 7)]
			log, _ := git(dir, "log", "--format=%h %s", mb+"..HEAD")
			all := lines(log)
			if len(all) > maxCommits {
				f.MoreCommits = len(all) - maxCommits
				all = all[:maxCommits]
			}
			f.Commits = append(f.Commits, all...)
			stat, _ := git(dir, "diff", "--stat=100", mb+"..HEAD")
			f.DiffStat = append(f.DiffStat, capKeepLast(lines(stat), maxDiffStat)...)
		}
	}
	if up, err := git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		f.Upstream = up
		if c, err := git(dir, "rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
			_, _ = fmt.Sscan(c, &f.Behind, &f.Ahead)
		}
	}
	st, _ := git(dir, "status", "--short", "--untracked-files=normal")
	f.Status = append(f.Status, capKeepLast(lines(st), maxStatus)...)
	return f, nil
}

// capKeepLast trims a list to n lines: the first n-2, a "… k more" line, and
// the last one (git's summary line).
func capKeepLast(ls []string, n int) []string {
	if len(ls) <= n {
		return ls
	}
	out := append([]string{}, ls[:n-2]...)
	out = append(out, fmt.Sprintf("… %d more", len(ls)-n+1))
	return append(out, ls[len(ls)-1])
}

// Markdown renders Facts as the text an agent receives at session start.
func (f Facts) Markdown() string {
	var b strings.Builder
	b.WriteString("# Loci handoff\n\n")
	b.WriteString("The previous context was cleared at a git boundary. These facts come from git;\n")
	b.WriteString("read the files you need instead of assuming earlier conversation.\n\n")
	fmt.Fprintf(&b, "- repo: %s\n", f.Repo)
	fmt.Fprintf(&b, "- branch: %s\n", f.Branch)
	if f.Issue != "" {
		// What was decided about the task lives in the tracker, not in git.
		fmt.Fprintf(&b, "- issue: %s (its text and decisions are not in this context; read it first)\n", f.Issue)
	}
	if f.Previous != "" {
		fmt.Fprintf(&b, "- previous branch: %s\n", f.Previous)
	}
	if f.Base != "" {
		fmt.Fprintf(&b, "- base: %s (merge-base %s)\n", f.Base, f.MergeBase)
	}
	if f.Upstream != "" {
		fmt.Fprintf(&b, "- upstream: %s (ahead %d, behind %d)\n", f.Upstream, f.Ahead, f.Behind)
	} else {
		b.WriteString("- upstream: none (not pushed)\n")
	}
	section(&b, fmt.Sprintf("Commits since base (%d)", len(f.Commits)+f.MoreCommits), f.Commits, f.MoreCommits)
	section(&b, "Changed files since base", f.DiffStat, 0)
	section(&b, "Uncommitted", f.Status, 0)
	s := b.String()
	if len(s) > maxChars {
		s = s[:maxChars] + "\n… (truncated)\n"
	}
	return s
}

func section(b *strings.Builder, title string, ls []string, more int) {
	if len(ls) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n\n", title)
	for _, l := range ls {
		fmt.Fprintf(b, "    %s\n", l)
	}
	if more > 0 {
		fmt.Fprintf(b, "    … %d older\n", more)
	}
}

// JSON renders Facts for agents that want structure.
func (f Facts) JSON() string {
	b, _ := json.MarshalIndent(f, "", "  ")
	return string(b) + "\n"
}
