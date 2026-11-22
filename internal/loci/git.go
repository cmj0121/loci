package loci

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs one git command in dir and returns its stdout without the trailing newline.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// A hook runs beside the agent's own git calls: never take optional locks.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// lines splits git output into its non-empty lines.
func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// currentBranch is the checked-out branch, or "" on a detached HEAD.
func currentBranch(dir string) (string, error) { return git(dir, "branch", "--show-current") }

// defaultBase is the branch work is based on: `git config loci.base`, then
// origin/main, origin/master, origin's HEAD, main, master. origin/HEAD comes
// after the named branches because a local clone copies whatever branch its
// source had checked out.
func defaultBase(dir string) string {
	if b, err := git(dir, "config", "--get", "loci.base"); err == nil && strings.TrimSpace(b) != "" {
		return strings.TrimSpace(b)
	}
	exists := func(ref string) bool {
		_, err := git(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		return err == nil
	}
	for _, b := range []string{"origin/main", "origin/master"} {
		if exists(b) {
			return b
		}
	}
	if ref, err := git(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && ref != "" {
		return ref
	}
	for _, b := range []string{"main", "master"} {
		if exists(b) {
			return b
		}
	}
	return ""
}
