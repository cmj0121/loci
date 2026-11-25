package loci

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// output decodes a hook's raw JSON output.
func output(t *testing.T, out string) hookOutput {
	t.Helper()
	var o hookOutput
	if out == "" {
		return o
	}
	if err := json.Unmarshal([]byte(out), &o); err != nil {
		t.Fatalf("hook output %q: %v", out, err)
	}
	return o
}

// suggest runs the switch of pendingRepo at the given nudge level and returns the suggestion.
func suggestAt(t *testing.T, level string) hookOutput {
	t.Helper()
	dir := newRepo(t)
	if level != "" {
		sh(t, dir, "git", "config", "loci.nudge", level)
	}
	in := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": transcript(t, 250_000)}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/a")
	hook(t, "post-tool-use", in)
	sh(t, dir, "git", "switch", "-q", "-c", "feat/b")
	return output(t, hook(t, "post-tool-use", in))
}

func TestNudgeLevelsAtTheSuggestion(t *testing.T) {
	for _, c := range []struct {
		level  string
		notice bool
		ask    bool
	}{
		{"", true, false}, // gate, the default
		{"gate", true, false},
		{"notice", true, false},
		{"ask", true, true},
		{"off", false, false},
		{"bogus", true, false},
	} {
		o := suggestAt(t, c.level)
		if o.HookSpecificOutput == nil || !strings.Contains(o.HookSpecificOutput.AdditionalContext, "/clear") {
			t.Fatalf("%q: the agent was not told: %+v", c.level, o)
		}
		if got := o.SystemMessage != ""; got != c.notice {
			t.Errorf("%q: notice %v, want %v", c.level, got, c.notice)
		}
		if got := strings.Contains(o.HookSpecificOutput.AdditionalContext, "AskUserQuestion"); got != c.ask {
			t.Errorf("%q: ask %v, want %v", c.level, got, c.ask)
		}
		if o.Decision != "" {
			t.Errorf("%q: PostToolUse blocked", c.level)
		}
	}
}

func TestStopRemindsOnceAndLetsTheTurnEnd(t *testing.T) {
	dir := pendingRepo(t)
	in := map[string]string{"session_id": "s1", "cwd": dir}
	raw := hook(t, "stop", in)
	o := output(t, raw)
	if !strings.Contains(o.SystemMessage, "fix/gh-2-b") || !strings.Contains(o.SystemMessage, "/clear") {
		t.Errorf("reminder = %q", o.SystemMessage)
	}
	if strings.Contains(raw, "hookSpecificOutput") || strings.Contains(raw, "decision") {
		t.Errorf("stop output would keep the turn going: %s", raw)
	}
	if out := hook(t, "stop", in); out != "" {
		t.Errorf("second stop said %q", out)
	}
	if ls := logLines(t, dir); ls[len(ls)-1]["message"] != msgReminded {
		t.Errorf("log = %v", ls[len(ls)-1])
	}
}

func TestStopStaysQuiet(t *testing.T) {
	for name, in := range map[string]map[string]string{
		"another session":  {"session_id": "s9"},
		"stop hook active": {"session_id": "s1", "stop_hook_active": "true"},
		"subagent":         {"session_id": "s1", "agent_id": "a1"},
	} {
		dir := pendingRepo(t)
		in["cwd"] = dir
		b, _ := json.Marshal(in)
		payload := strings.Replace(string(b), `"true"`, `true`, 1)
		if _, out, _ := run(t, payload, "hook", "stop"); out != "" {
			t.Errorf("%s: said %q", name, out)
		}
	}
	dir := pendingRepo(t)
	sh(t, dir, "git", "config", "loci.nudge", "off")
	if out := hook(t, "stop", map[string]string{"session_id": "s1", "cwd": dir}); out != "" {
		t.Errorf("nudge off: said %q", out)
	}
}

func TestGateHoldsTheNextPromptOnce(t *testing.T) {
	dir := pendingRepo(t)
	in := func(p string) map[string]string {
		return map[string]string{"session_id": "s1", "cwd": dir, "prompt": p}
	}
	for _, p := range []string{"/clear", "/compact", "<task-notification>done</task-notification>", ""} {
		if out := hook(t, "user-prompt-submit", in(p)); out != "" {
			t.Errorf("%q was held: %s", p, out)
		}
	}
	o := output(t, hook(t, "user-prompt-submit", in("now fix the tests")))
	if o.Decision != "block" || !strings.Contains(o.Reason, "fix/gh-2-b") || !strings.Contains(o.Reason, "send your prompt again") {
		t.Fatalf("gate = %+v", o)
	}
	if o.HookSpecificOutput != nil {
		t.Errorf("the gate told the agent: %+v", o.HookSpecificOutput)
	}
	if out := hook(t, "user-prompt-submit", in("now fix the tests")); out != "" {
		t.Errorf("second send was held: %s", out)
	}
	st, _ := loadState(filepath.Join(dir, ".git"))
	if st.Pending == nil || !st.Pending.Gated {
		t.Errorf("pending = %+v", st.Pending)
	}
	if ls := logLines(t, dir); ls[len(ls)-1]["message"] != msgGated {
		t.Errorf("log = %v", ls[len(ls)-1])
	}
}

func TestGateLetsPromptsThrough(t *testing.T) {
	cases := map[string]func(dir string) map[string]string{
		"another session": func(dir string) map[string]string {
			return map[string]string{"session_id": "s9", "cwd": dir, "prompt": "hi"}
		},
		"nudge notice": func(dir string) map[string]string {
			sh(t, dir, "git", "config", "loci.nudge", "notice")
			return map[string]string{"session_id": "s1", "cwd": dir, "prompt": "hi"}
		},
		"nudge ask": func(dir string) map[string]string {
			sh(t, dir, "git", "config", "loci.nudge", "ask")
			return map[string]string{"session_id": "s1", "cwd": dir, "prompt": "hi"}
		},
	}
	for name, mk := range cases {
		dir := pendingRepo(t)
		if out := hook(t, "user-prompt-submit", mk(dir)); out != "" {
			t.Errorf("%s: held %s", name, out)
		}
	}
	dir := newRepo(t)
	if out := hook(t, "user-prompt-submit", map[string]string{"session_id": "s1", "cwd": dir, "prompt": "hi"}); out != "" {
		t.Errorf("no handoff: held %s", out)
	}
	dir = pendingRepo(t)
	defer func(f func() time.Time) { now = f }(now)
	at := time.Now()
	now = func() time.Time { return at.Add(pendingTTL + time.Minute) }
	if out := hook(t, "user-prompt-submit", map[string]string{"session_id": "s1", "cwd": dir, "prompt": "hi"}); out != "" {
		t.Errorf("stale handoff: held %s", out)
	}
}
