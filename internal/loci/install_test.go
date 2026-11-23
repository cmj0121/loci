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
	if c := commands(readSettings(t, dir), "SubagentStop"); len(c) != 1 || !strings.HasSuffix(c[0], " hook subagent-stop") {
		t.Errorf("SubagentStop = %v", c)
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

func writeSettings(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func statusLine(t *testing.T, repo string) map[string]any {
	t.Helper()
	sl, _ := readSettings(t, repo)["statusLine"].(map[string]any)
	return sl
}

func TestInstallStatusLineWithOnlyUserOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"statusLine":{"type":"command","command":"my-status"}}`)
	repo := t.TempDir()
	exe := "/usr/local/bin/loci"
	if _, changed, err := Install(repo, exe, false); err != nil || !changed {
		t.Fatalf("install: changed %v, %v", changed, err)
	}
	sl := statusLine(t, repo)
	if len(sl) != 2 || sl["type"] != "command" || sl["command"] != "/usr/local/bin/loci statusline" {
		t.Errorf("statusLine = %v", sl)
	}
	if _, changed, _ := Install(repo, exe, false); changed {
		t.Error("a second install changed the file")
	}
	if _, _, err := Install(repo, exe, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := readSettings(t, repo)["statusLine"]; ok {
		t.Error("uninstall kept Loci's statusLine")
	}
}

func TestInstallWrapsAndRestoresLocalStatusLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, prev := range []string{"my-status", `printf '%s' "it's $PWD"`, "a|b", "~/bin/s*"} {
		repo := t.TempDir()
		body, _ := json.Marshal(map[string]any{
			"permissions": map[string]any{"allow": []any{"Bash"}},
			"hooks":       map[string]any{"PostToolUse": []any{map[string]any{"matcher": "Edit", "hooks": []any{map[string]any{"type": "command", "command": "fmt"}}}}},
			"statusLine":  map[string]any{"type": "command", "command": prev, "padding": 1},
		})
		writeSettings(t, settingsPath(repo), string(body))
		exe := "/opt/my tools/loci"
		if _, _, err := Install(repo, exe, false); err != nil {
			t.Fatal(err)
		}
		sl := statusLine(t, repo)
		want := "'/opt/my tools/loci' statusline --then " + shellQuote(prev)
		if sl["command"] != want || sl["padding"] != 1.0 || len(sl) != 3 {
			t.Errorf("statusLine = %v, want command %q", sl, want)
		}
		if _, changed, _ := Install(repo, exe, false); changed {
			t.Error("re-install wrapped Loci's own status line")
		}
		if _, _, err := Install(repo, exe, true); err != nil {
			t.Fatal(err)
		}
		m := readSettings(t, repo)
		sl, _ = m["statusLine"].(map[string]any)
		if sl["command"] != prev || sl["type"] != "command" || sl["padding"] != 1.0 || len(sl) != 3 {
			t.Errorf("restored statusLine = %v, want %q", sl, prev)
		}
		if m["permissions"] == nil || strings.Join(commands(m, "PostToolUse"), "|") != "fmt" {
			t.Errorf("other settings changed: %v", m)
		}
	}
}

func TestUninstallLeavesOtherStatusLine(t *testing.T) {
	repo := t.TempDir()
	writeSettings(t, settingsPath(repo), `{"statusLine":{"type":"command","command":"my-status"}}`)
	if _, _, err := Install(repo, "/usr/local/bin/loci", true); err != nil {
		t.Fatal(err)
	}
	if sl := statusLine(t, repo); sl["command"] != "my-status" {
		t.Errorf("statusLine = %v", sl)
	}
}

func TestShellUnquoteInvertsShellQuote(t *testing.T) {
	for _, s := range []string{"my-status", "/opt/my tools/x", `it's "a" $b`, "''", "a statusline --then b", "a|b", "~/bin/s*", "x;y", "cat>f"} {
		if got := shellUnquote(shellQuote(s)); got != s {
			t.Errorf("shellUnquote(shellQuote(%q)) = %q", s, got)
		}
	}
}

func TestOursStatusAnchored(t *testing.T) {
	for c, want := range map[string]bool{
		"loci statusline": true,
		"/usr/local/bin/loci statusline --then 'x'": true,
		"'/opt/my loci/loci' statusline":            true,
		"'loci' statusline":                         true,
		"/tmp/x/loci.test statusline":               true,
		"x; loci statusline":                        false,
		"echo loci statusline":                      false,
		"my-status; /usr/local/bin/loci statusline": false,
		"x;/usr/local/bin/loci statusline":          false,
		"/home/loci/bin/other statusline":           false,
		"/usr/local/bin/loci statuslinex":           false,
	} {
		if got := oursStatus.MatchString(c); got != want {
			t.Errorf("oursStatus(%q) = %v", c, got)
		}
	}
}

func TestOursStatusMatchesShellQuotedExe(t *testing.T) {
	for _, exe := range []string{
		"/usr/local/bin/loci",
		"/opt/my tools/loci",
		"/home/使用者/bin/loci",
		"/a/it's here/loci",
		"/tmp/x/loci.test",
	} {
		for _, c := range []string{shellQuote(exe) + " statusline", shellQuote(exe) + thenFlag + shellQuote("echo x")} {
			if !oursStatus.MatchString(c) {
				t.Errorf("oursStatus(%q) = false", c)
			}
		}
	}
}
