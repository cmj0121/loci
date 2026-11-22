package loci

import (
	"path"
	"strconv"
	"strings"
)

// DefaultThreshold is the context size, in tokens, at which a switch to a new
// branch becomes worth a handoff. Override it with `git config loci.threshold`.
const DefaultThreshold = 100_000

// defaultSkip are long-lived branches: switching to one is a look or a rebase,
// not a new task. Add more with `git config --add loci.skip <glob>`.
var defaultSkip = []string{"main", "master", "develop", "dev", "stage", "staging", "pre-release", "release", "release/*", "*production*"}

// Config is the rule's settings for one repository.
type Config struct {
	Threshold int      // context tokens at which a switch is a handoff
	Skip      []string // globs of long-lived branches, the base branch included
}

// defaultConfig is the rule without a repository to read settings from.
func defaultConfig() Config {
	return Config{Threshold: DefaultThreshold, Skip: append([]string{}, defaultSkip...)}
}

// loadConfig reads loci.threshold, loci.skip and the base branch of the repository at dir.
func loadConfig(dir string) Config {
	c := defaultConfig()
	if v, err := git(dir, "config", "--get", "loci.threshold"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			c.Threshold = n
		}
	}
	if extra, err := git(dir, "config", "--get-all", "loci.skip"); err == nil {
		for _, p := range lines(extra) {
			c.Skip = append(c.Skip, strings.TrimSpace(p))
		}
	}
	if b := defaultBase(dir); b != "" {
		c.Skip = append(c.Skip, strings.TrimPrefix(b, "origin/"))
	}
	return c
}

// longLived reports whether br matches one of the skip globs.
func (c Config) longLived(br string) bool {
	for _, p := range c.Skip {
		if ok, _ := path.Match(p, br); ok {
			return true
		}
	}
	return false
}
