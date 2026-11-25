package loci

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// transcript writes a transcript whose last main-chain turn has ctx tokens of context.
func transcript(t *testing.T, ctx int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	ls := []string{
		`{"type":"user","message":{"content":"hi"}}`,
		fmt.Sprintf(`{"type":"assistant","message":{"usage":{"input_tokens":3,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`, ctx/4, ctx-ctx/4-3),
		`{"type":"assistant","isSidechain":true,"message":{"usage":{"input_tokens":1,"cache_read_input_tokens":9}}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result"}]}}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// hook runs one hook subcommand with a payload built from fields.
func hook(t *testing.T, sub string, fields map[string]string) string {
	t.Helper()
	b, _ := json.Marshal(fields)
	code, out, errs := run(t, string(b), "hook", sub)
	if code != 0 || errs != "" {
		t.Fatalf("hook %s exit %d stderr %q", sub, code, errs)
	}
	return out
}

// notice is the systemMessage Claude Code shows the user.
func notice(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var o hookOutput
	if err := json.Unmarshal([]byte(out), &o); err != nil {
		t.Fatalf("hook output %q: %v", out, err)
	}
	return o.SystemMessage
}

func additional(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var o hookOutput
	if err := json.Unmarshal([]byte(out), &o); err != nil {
		t.Fatalf("hook output %q: %v", out, err)
	}
	return o.HookSpecificOutput.AdditionalContext
}

// logLines reads the repository's Loci log as JSON objects.
func logLines(t *testing.T, dir string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, ".git", "loci", "log"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []map[string]any
	s := bufio.NewScanner(f)
	for s.Scan() {
		var m map[string]any
		if err := json.Unmarshal(s.Bytes(), &m); err != nil {
			t.Fatalf("log line %q: %v", s.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

func TestPostToolUseSuggestsAtANewBranch(t *testing.T) {
	dir := newRepo(t)
	tp := transcript(t, 250_000)
	in := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": tp, "tool_name": "Bash"}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/gh-1-a")
	if out := hook(t, "post-tool-use", in); out != "" {
		t.Fatalf("first sight said %q", out)
	}
	if out := hook(t, "post-tool-use", in); out != "" {
		t.Fatalf("same branch said %q", out)
	}
	sh(t, dir, "git", "switch", "-q", "-c", "fix/gh-2-b")
	out := hook(t, "post-tool-use", in)
	msg := additional(t, out)
	for _, want := range []string{"feat/gh-1-a to fix/gh-2-b", "250k tokens", "/clear"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message misses %q: %s", want, msg)
		}
	}
	for _, want := range []string{"fix/gh-2-b", "250k tokens", "Run /clear"} {
		if n := notice(t, out); !strings.Contains(n, want) {
			t.Errorf("notice misses %q: %s", want, n)
		}
	}
	st, _ := loadState(filepath.Join(dir, ".git"))
	if st.Pending == nil || st.Pending.To != "fix/gh-2-b" || st.Pending.Context != 250_000 {
		t.Errorf("pending = %+v", st.Pending)
	}
	ls := logLines(t, dir)
	if len(ls) != 1 || ls[0]["message"] != msgBoundary || ls[0]["reason"] != ReasonSuggest || ls[0]["suggested"] != true {
		t.Errorf("log = %v", ls)
	}
}

func TestPostToolUseStaysQuiet(t *testing.T) {
	dir := newRepo(t)
	big, small := transcript(t, 300_000), transcript(t, 20_000)
	in := func(tp string) map[string]string {
		return map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": tp}
	}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/a")
	hook(t, "post-tool-use", in(big))
	for _, c := range []struct{ switchTo, tp, reason string }{
		{"-c feat/b", small, ReasonSmall},
		{"main", big, ReasonLongLived},
		{"feat/a", big, ReasonRevisit},
	} {
		sh(t, dir, append([]string{"git", "switch", "-q"}, strings.Fields(c.switchTo)...)...)
		if out := hook(t, "post-tool-use", in(c.tp)); out != "" {
			t.Errorf("%s said %q", c.switchTo, out)
		}
		ls := logLines(t, dir)
		if got := ls[len(ls)-1]["reason"]; got != c.reason {
			t.Errorf("%s logged reason %v, want %s", c.switchTo, got, c.reason)
		}
	}
}

func TestPostToolUseReadsSettingsFromGitConfig(t *testing.T) {
	dir := newRepo(t)
	sh(t, dir, "git", "config", "loci.threshold", "500000")
	sh(t, dir, "git", "config", "--add", "loci.skip", "spike/*")
	in := map[string]string{"session_id": "s", "cwd": dir, "transcript_path": transcript(t, 300_000)}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/a")
	hook(t, "post-tool-use", in)
	sh(t, dir, "git", "switch", "-q", "-c", "feat/b")
	hook(t, "post-tool-use", in)
	sh(t, dir, "git", "switch", "-q", "-c", "spike/c")
	hook(t, "post-tool-use", in)
	ls := logLines(t, dir)
	if ls[0]["reason"] != ReasonSmall || ls[0]["threshold"] != 500000.0 || ls[1]["reason"] != ReasonLongLived {
		t.Errorf("log = %v", ls)
	}
}

func TestPostToolUseSkipsTheBaseBranch(t *testing.T) {
	dir := newRepo(t)
	sh(t, dir, "git", "switch", "-q", "-c", "trunk")
	sh(t, dir, "git", "config", "loci.base", "trunk")
	sh(t, dir, "git", "switch", "-q", "-c", "feat/a")
	in := map[string]string{"session_id": "s", "cwd": dir, "transcript_path": transcript(t, 300_000)}
	hook(t, "post-tool-use", in)
	sh(t, dir, "git", "switch", "-q", "trunk")
	if out := hook(t, "post-tool-use", in); out != "" {
		t.Errorf("switch to the base said %q", out)
	}
}

func TestPostToolUseLogsAReplacedSuggestion(t *testing.T) {
	dir := newRepo(t)
	in := map[string]string{"session_id": "s", "cwd": dir, "transcript_path": transcript(t, 300_000)}
	for _, br := range []string{"feat/a", "feat/b", "feat/c"} {
		sh(t, dir, "git", "switch", "-q", "-c", br)
		hook(t, "post-tool-use", in)
	}
	ls := logLines(t, dir)
	if len(ls) != 3 || ls[1]["message"] != msgReplaced || ls[1]["to"] != "feat/b" {
		t.Errorf("log = %v", ls)
	}
}

func TestHooksNeverFail(t *testing.T) {
	isolateGit(t)
	dir := newRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "loci", "state.json"), 0o755); err != nil {
		t.Fatal(err) // a directory where the state file should be: every read fails
	}
	for _, stdin := range []string{"", "not json", `{"cwd":""}`, `{"cwd":"` + t.TempDir() + `"}`, `{"cwd":"` + dir + `"}`, `{"cwd":"` + dir + `","source":"clear"}`} {
		for _, sub := range []string{"post-tool-use", "session-start"} {
			code, out, errs := run(t, stdin, "hook", sub)
			if code != 0 || out != "" || errs != "" {
				t.Errorf("%s %q: exit %d out %q err %q", sub, stdin, code, out, errs)
			}
		}
	}
	ls := logLines(t, dir)
	if len(ls) == 0 || ls[len(ls)-1]["message"] != msgFailed {
		t.Errorf("the failure was not logged: %v", ls)
	}
}

func TestContextTokens(t *testing.T) {
	if n, err := ContextTokens(transcript(t, 123_456)); err != nil || n != 123_456 {
		t.Errorf("ContextTokens = %d, %v", n, err)
	}
	if n, err := ContextTokens(""); err != nil || n != 0 {
		t.Errorf("no transcript = %d, %v", n, err)
	}
	if _, err := ContextTokens(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing transcript is not an error")
	}
	// a long tail of records without usage: the window has to grow past its first read
	p := transcript(t, 42_000)
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	filler := `{"type":"user","message":{"content":"` + strings.Repeat("x", 1000) + `"}}` + "\n"
	for range (tailChunk / len(filler)) * 3 {
		_, _ = f.WriteString(filler)
	}
	_ = f.Close()
	if n, err := ContextTokens(p); err != nil || n != 42_000 {
		t.Errorf("past the first window = %d, %v", n, err)
	}
}

func TestCorruptStateIsReset(t *testing.T) {
	gd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gd, "loci"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(gd), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadState(gd)
	if err != nil || st.Branch != "" {
		t.Errorf("corrupt state = %+v, %v", st, err)
	}
}

// pendingRepo is a repository with a suggestion waiting on fix/gh-2-b.
func pendingRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	in := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": transcript(t, 250_000)}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/gh-1-a")
	hook(t, "post-tool-use", in)
	sh(t, dir, "git", "switch", "-q", "-c", "fix/gh-2-b")
	commitFile(t, dir, "b.go", "package b", "fix(gh-2): b")
	if additional(t, hook(t, "post-tool-use", in)) == "" {
		t.Fatal("no suggestion")
	}
	return dir
}

func TestSessionStartInjectsTheHandoffOnce(t *testing.T) {
	dir := pendingRepo(t)
	md := additional(t, hook(t, "session-start", map[string]string{"session_id": "s2", "cwd": dir, "source": "clear"}))
	for _, want := range []string{"# Loci handoff", "- branch: fix/gh-2-b", "- issue: gh-2", "- previous branch: feat/gh-1-a", "fix(gh-2): b"} {
		if !strings.Contains(md, want) {
			t.Errorf("handoff misses %q:\n%s", want, md)
		}
	}
	if out := hook(t, "session-start", map[string]string{"session_id": "s3", "cwd": dir, "source": "clear"}); out != "" {
		t.Errorf("second session got %q", out)
	}
	ls := logLines(t, dir)
	last := ls[len(ls)-1]
	if last["message"] != msgHandoff || last["context"] != 250000.0 || last["source"] != "clear" || last["chars"] != float64(len(md)) {
		t.Errorf("log = %v", last)
	}
}

func TestSessionStartTellsTheUser(t *testing.T) {
	dir := pendingRepo(t)
	out := hook(t, "session-start", map[string]string{"session_id": "s2", "cwd": dir, "source": "clear"})
	if n := notice(t, out); !strings.Contains(n, "fix/gh-2-b") {
		t.Errorf("notice = %q", n)
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Sessions["s1"] != nil {
		t.Error("the cleared session is still remembered")
	}
}

func TestSessionStartBesideALiveSessionKeepsTheHandoff(t *testing.T) {
	dir := pendingRepo(t) // s1 crossed the boundary just now
	if md := additional(t, hook(t, "session-start", map[string]string{"session_id": "s9", "cwd": dir, "source": "startup"})); md == "" {
		t.Fatal("the new session got no copy")
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Pending == nil {
		t.Fatal("a session beside s1 took its handoff")
	}
	if ls := logLines(t, dir); ls[len(ls)-1]["kept"] != true {
		t.Errorf("log = %v", ls[len(ls)-1])
	}
	if additional(t, hook(t, "session-start", map[string]string{"session_id": "s2", "cwd": dir, "source": "clear"})) == "" {
		t.Error("s1's /clear got nothing")
	}
}

func TestSessionStartAfterTheSessionWentQuietTakesTheHandoff(t *testing.T) {
	dir := pendingRepo(t)
	defer func(f func() time.Time) { now = f }(now)
	at := time.Now()
	now = func() time.Time { return at.Add(liveFor + time.Minute) }
	if additional(t, hook(t, "session-start", map[string]string{"session_id": "s9", "cwd": dir, "source": "startup"})) == "" {
		t.Fatal("startup got nothing")
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Pending != nil {
		t.Error("the handoff of a quiet session was kept")
	}
}

func TestLegacyStateKeepsWhatTheSessionVisited(t *testing.T) {
	gd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gd, "loci"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := `{"branch":"feat/b","session":"s1","visited":["feat/a"]}`
	if err := os.WriteFile(statePath(gd), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadState(gd)
	if err != nil || st.Branch != "feat/b" || st.Sessions["s1"] == nil || st.Sessions["s1"].Visited[0] != "feat/a" {
		t.Fatalf("legacy state = %+v, %v", st, err)
	}
	if v := see(&st, "s1", "feat/a", 300_000); v.Reason != ReasonRevisit {
		t.Errorf("after the upgrade: %q", v.Reason)
	}
}

func TestSessionStartIgnoresResumeAndCompact(t *testing.T) {
	dir := pendingRepo(t)
	for _, src := range []string{"resume", "compact", ""} {
		if out := hook(t, "session-start", map[string]string{"cwd": dir, "source": src}); out != "" {
			t.Errorf("source %q got %q", src, out)
		}
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Pending == nil {
		t.Error("the pending handoff was consumed")
	}
	if out := hook(t, "session-start", map[string]string{"cwd": dir, "source": "startup"}); out == "" {
		t.Error("startup got nothing")
	}
}

func TestSessionStartDropsAStaleHandoff(t *testing.T) {
	dir := pendingRepo(t)
	defer func(f func() time.Time) { now = f }(now)
	at := time.Now()
	now = func() time.Time { return at.Add(pendingTTL + time.Minute) }
	if out := hook(t, "session-start", map[string]string{"cwd": dir, "source": "clear"}); out != "" {
		t.Errorf("stale handoff injected: %q", out)
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Pending != nil {
		t.Error("stale handoff kept")
	}
	if ls := logLines(t, dir); ls[len(ls)-1]["message"] != msgStale {
		t.Errorf("log = %v", ls[len(ls)-1])
	}
}

func TestHandoffNamesThePendingPreviousBranch(t *testing.T) {
	dir := pendingRepo(t)
	code, out, _ := run(t, "", "handoff", "-C", dir)
	if code != 0 || !strings.Contains(out, "- previous branch: feat/gh-1-a") {
		t.Errorf("exit %d:\n%s", code, out)
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Pending == nil {
		t.Error("loci handoff consumed the pending handoff")
	}
}

// subagentTranscript writes a subagent's own transcript, whose turns are all
// sidechain, ending with ctx tokens of context.
func subagentTranscript(t *testing.T, ctx int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "subagents", "agent-a1.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	ls := []string{
		`{"type":"user","isSidechain":true,"agentId":"a1","message":{"content":"look"}}`,
		`{"type":"assistant","isSidechain":true,"agentId":"a1","message":{"usage":{"input_tokens":5,"cache_read_input_tokens":100}}}`,
		fmt.Sprintf(`{"type":"assistant","isSidechain":true,"agentId":"a1","message":{"usage":{"input_tokens":2,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`, ctx/4, ctx-ctx/4-2),
	}
	if err := os.WriteFile(p, []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSubagentTokens(t *testing.T) {
	p := subagentTranscript(t, 40_000)
	if n, err := SubagentTokens(p); err != nil || n != 40_000 {
		t.Errorf("SubagentTokens = %d, %v", n, err)
	}
	if n, _ := ContextTokens(p); n != 0 {
		t.Errorf("ContextTokens read a sidechain turn: %d", n)
	}
}

func TestPostToolUseIgnoresSubagents(t *testing.T) {
	dir := newRepo(t)
	tp := transcript(t, 250_000)
	main := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": tp, "tool_name": "Bash"}
	sub := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": tp, "tool_name": "Bash",
		"agent_id": "a1", "agent_type": "general-purpose"}
	sh(t, dir, "git", "switch", "-q", "-c", "feat/a")
	hook(t, "post-tool-use", main)
	sh(t, dir, "git", "switch", "-q", "-c", "feat/b")
	if out := hook(t, "post-tool-use", sub); out != "" {
		t.Fatalf("a subagent was told %q", additional(t, out))
	}
	if st, _ := loadState(filepath.Join(dir, ".git")); st.Branch != "feat/a" || st.Pending != nil {
		t.Errorf("a subagent's call moved the state: %+v", st)
	}
	if ls := logLines(t, dir); len(ls) != 0 {
		t.Errorf("a subagent's call was logged: %v", ls)
	}
	// The main session's next command judges the switch the subagent made.
	if msg := additional(t, hook(t, "post-tool-use", main)); !strings.Contains(msg, "feat/a to feat/b") {
		t.Errorf("main session missed the switch: %q", msg)
	}
}

func TestSubagentStopLogsItsContext(t *testing.T) {
	dir := newRepo(t)
	in := map[string]string{"session_id": "s1", "cwd": dir, "transcript_path": transcript(t, 250_000),
		"agent_id": "a1", "agent_type": "Explore", "agent_transcript_path": subagentTranscript(t, 40_000)}
	if out := hook(t, "subagent-stop", in); out != "" {
		t.Errorf("subagent-stop said %q", out)
	}
	ls := logLines(t, dir)
	if len(ls) != 1 || ls[0]["message"] != msgSubagent || ls[0]["agent"] != "a1" || ls[0]["type"] != "Explore" || ls[0]["context"] != 40_000.0 {
		t.Fatalf("log = %v", ls)
	}
	// Not from a subagent, no transcript, or an empty one: nothing is logged.
	for _, drop := range []string{"agent_id", "agent_transcript_path"} {
		m := maps.Clone(in)
		delete(m, drop)
		hook(t, "subagent-stop", m)
	}
	in["agent_transcript_path"] = filepath.Join(t.TempDir(), "missing.jsonl")
	hook(t, "subagent-stop", in)
	if ls := logLines(t, dir); len(ls) != 2 || ls[1]["level"] != "warn" {
		t.Errorf("log = %v", ls)
	}
}
