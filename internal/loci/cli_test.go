package loci

import (
	"bytes"
	"strings"
	"testing"
)

// run calls the CLI with captured streams.
func run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, "", "--version")
	if code != 0 || strings.TrimSpace(out) != Version {
		t.Fatalf("--version = %d %q, want 0 %q", code, out, Version)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	code, out, _ := run(t, "", "--help")
	if code != 0 {
		t.Fatalf("--help exit %d", code)
	}
	for _, c := range []string{"hook post-tool-use", "hook session-start", "handoff", "install", "stats", "replay"} {
		if !strings.Contains(out, c) {
			t.Errorf("--help misses %q:\n%s", c, out)
		}
	}
}

func TestUsageErrorExitsTwo(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"handoff", "--format", "xml"}, {}} {
		if code, _, _ := run(t, "", args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
}
