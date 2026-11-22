package loci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSettings(t *testing.T, repo string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(settingsPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// commands lists the hook commands of one event.
func commands(m map[string]any, event string) []string {
	var out []string
	hooks, _ := m["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	for _, g := range groups {
		hs, _ := g.(map[string]any)["hooks"].([]any)
		for _, h := range hs {
			out = append(out, h.(map[string]any)["command"].(string))
		}
	}
	return out
}

func TestInstallKeepsOtherSettingsAndIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := `{"permissions":{"allow":["Bash"]},"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"fmt"}]}],
	"SessionStart":[{"hooks":[{"type":"command","command":"/old/loci hook session-start"}]}]}}`
	if err := os.WriteFile(settingsPath(repo), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := "/opt/my tools/loci"
	if _, changed, err := Install(repo, exe, false); err != nil || !changed {
		t.Fatalf("install: changed %v, %v", changed, err)
	}
	m := readSettings(t, repo)
	if m["permissions"] == nil {
		t.Error("permissions lost")
	}
	if got := strings.Join(commands(m, "PostToolUse"), "|"); got != "fmt|'/opt/my tools/loci' hook post-tool-use" {
		t.Errorf("PostToolUse = %s", got)
	}
	if got := strings.Join(commands(m, "SessionStart"), "|"); got != "'/opt/my tools/loci' hook session-start" {
		t.Errorf("SessionStart = %s (the old entry should be replaced)", got)
	}
	if _, changed, _ := Install(repo, exe, false); changed {
		t.Error("a second install changed the file")
	}
	if _, changed, err := Install(repo, exe, true); err != nil || !changed {
		t.Fatalf("uninstall: changed %v, %v", changed, err)
	}
	m = readSettings(t, repo)
	if got := commands(m, "PostToolUse"); len(got) != 1 || got[0] != "fmt" {
		t.Errorf("uninstall left %v", got)
	}
	if _, ok := m["hooks"].(map[string]any)["SessionStart"]; ok {
		t.Error("an empty SessionStart was kept")
	}
}

func TestUninstallWithoutSettingsWritesNothing(t *testing.T) {
	repo := t.TempDir()
	if _, changed, err := Install(repo, "loci", true); err != nil || changed {
		t.Errorf("changed %v, %v", changed, err)
	}
	if _, err := os.Stat(settingsPath(repo)); err == nil {
		t.Error("settings file created")
	}
}

func TestInstallRejectsBrokenSettings(t *testing.T) {
	repo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	_ = os.WriteFile(settingsPath(repo), []byte("{"), 0o644)
	if _, _, err := Install(repo, "loci", false); err == nil {
		t.Error("broken settings overwritten")
	}
}

func TestInstallCommand(t *testing.T) {
	dir := newRepo(t)
	code, out, errs := run(t, "", "install", "-C", dir)
	if code != 0 || !strings.Contains(out, "installed in") {
		t.Fatalf("exit %d out %q err %q", code, out, errs)
	}
	if c := commands(readSettings(t, dir), "SessionStart"); len(c) != 1 || !strings.HasSuffix(c[0], " hook session-start") {
		t.Errorf("SessionStart = %v", c)
	}
	if _, out, _ = run(t, "", "install", "-C", dir); !strings.Contains(out, "unchanged") {
		t.Errorf("second install: %q", out)
	}
	if _, out, _ = run(t, "", "install", "-C", dir, "--uninstall"); !strings.Contains(out, "removed from") {
		t.Errorf("uninstall: %q", out)
	}
}

func TestOursMatchesOnlyLociHooks(t *testing.T) {
	for c, want := range map[string]bool{
		"loci hook post-tool-use":                 true,
		"/usr/local/bin/loci hook session-start":  true,
		"'/opt/my tools/loci' hook post-tool-use": true,
		"/tmp/x/loci.test hook session-start":     true,
		"loci hook prompt":                        false, // only the two hooks Loci installs are matched
		"my-linter hook post-tool-use --fix":      false,
		"/home/loci/bin/other hook post-tool-use": false,
	} {
		if got := ours.MatchString(c); got != want {
			t.Errorf("ours(%q) = %v", c, got)
		}
	}
}
