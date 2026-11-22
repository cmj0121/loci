package loci

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ours matches the hook commands Loci owns: its binary, quoted or not, then a hook subcommand.
var ours = regexp.MustCompile(`loci[^/\s']*'? hook (post-tool-use|session-start)$`)

// oursStatus matches Loci's own status line command, so it never wraps itself.
var oursStatus = regexp.MustCompile(`loci[^/\s']*'? statusline( |$)`)

// settingsPath is the repository-local Claude Code settings file, not shared with the team.
func settingsPath(repo string) string { return filepath.Join(repo, ".claude", "settings.local.json") }

// hookEvents are the Claude Code events Loci hooks, with its subcommand and matcher.
var hookEvents = []struct{ event, sub, matcher string }{
	{"PostToolUse", "post-tool-use", "Bash"}, // a branch only changes through a shell command
	{"SessionStart", "session-start", ""},
}

// Install adds (or, with remove, removes) Loci's hooks and status line in the
// repository's local settings. It keeps every other key and hook, replaces an
// older Loci entry, wraps (and on removal restores) a local status line it
// finds, and reports whether the file changed.
func Install(repo, exe string, remove bool) (path string, changed bool, err error) {
	path = settingsPath(repo)
	data := map[string]any{}
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return path, false, err
	default:
		if err := json.Unmarshal(old, &data); err != nil {
			return path, false, err
		}
	}
	hooks, _ := data["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, h := range hookEvents {
		groups := dropOurs(hooks[h.event])
		if !remove {
			g := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": shellQuote(exe) + " hook " + h.sub}}}
			if h.matcher != "" {
				g["matcher"] = h.matcher
			}
			groups = append(groups, g)
		}
		if len(groups) == 0 {
			delete(hooks, h.event)
		} else {
			hooks[h.event] = groups
		}
	}
	if len(hooks) == 0 {
		delete(data, "hooks")
	} else {
		data["hooks"] = hooks
	}
	setStatusLine(data, exe, remove)
	if old == nil && remove {
		return path, false, nil
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return path, false, err
	}
	b = append(b, '\n')
	if string(old) == string(b) {
		return path, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, false, err
	}
	return path, true, os.WriteFile(path, b, 0o644)
}

// thenFlag separates Loci's status line command from the one it wraps.
const thenFlag = " statusline --then "

// setStatusLine points the local statusLine at Loci, wrapping a non-Loci one
// through --then, or with remove undoes that. Another tool's line is never removed.
func setStatusLine(data map[string]any, exe string, remove bool) {
	sl, _ := data["statusLine"].(map[string]any)
	cmd, _ := sl["command"].(string)
	mine := oursStatus.MatchString(cmd)
	then := ""
	switch {
	case mine:
		if i := strings.Index(cmd, thenFlag); i >= 0 {
			then = shellUnquote(cmd[i+len(thenFlag):])
		}
	case cmd != "" && !remove:
		then = cmd
	}
	next := map[string]any{"type": "command"}
	switch {
	case remove && !mine:
		return
	case remove && then == "":
		delete(data, "statusLine")
		return
	case remove:
		next["command"] = then
	case then == "":
		next["command"] = shellQuote(exe) + " statusline"
	default:
		next["command"] = shellQuote(exe) + thenFlag + shellQuote(then)
	}
	if p, ok := sl["padding"]; ok {
		next["padding"] = p
	}
	data["statusLine"] = next
}

// dropOurs removes Loci's commands from a hook event's groups, and groups left empty.
func dropOurs(v any) []any {
	groups, _ := v.([]any)
	var out []any
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			out = append(out, g)
			continue
		}
		hs, _ := gm["hooks"].([]any)
		var keep []any
		for _, h := range hs {
			if hm, ok := h.(map[string]any); ok {
				if c, _ := hm["command"].(string); ours.MatchString(c) {
					continue
				}
			}
			keep = append(keep, h)
		}
		if len(keep) == 0 {
			continue
		}
		gm["hooks"] = keep
		out = append(out, gm)
	}
	return out
}

// shellSafe matches words the shell reads literally, so they need no quotes.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes s for a shell command line unless it is a safe bare word.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellUnquote inverts shellQuote.
func shellUnquote(s string) string {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return s
	}
	return strings.ReplaceAll(s[1:len(s)-1], `'\''`, "'")
}

// previousStatus is the status line command Loci wraps when no --then was
// recorded: the project's, else the user's, skipping one of Loci's own.
func previousStatus(top string) string {
	var paths []string
	if top != "" {
		paths = append(paths, filepath.Join(top, ".claude", "settings.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".claude", "settings.json"))
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s struct {
			StatusLine struct {
				Command string `json:"command"`
			} `json:"statusLine"`
		}
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		if c := s.StatusLine.Command; c != "" && !oursStatus.MatchString(c) {
			return c
		}
	}
	return ""
}
