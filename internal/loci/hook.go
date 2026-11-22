package loci

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// HookInput is the subset of the Claude Code hook payload Loci reads.
type HookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name"`
	Source         string `json:"source"` // SessionStart: startup, resume, clear, compact
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// worktree is where a hook runs: the worktree's .git directory and its branch.
type worktree struct{ gitDir, branch string }

// locate finds the worktree at dir with one git call; branch is "" on a detached HEAD.
func locate(dir string) (worktree, error) {
	out, err := git(dir, "rev-parse", "--absolute-git-dir", "--abbrev-ref", "HEAD")
	if err != nil {
		return worktree{}, err
	}
	ls := strings.Split(out, "\n")
	if len(ls) != 2 {
		return worktree{}, fmt.Errorf("git rev-parse: unexpected output %q", out)
	}
	w := worktree{gitDir: ls[0], branch: ls[1]}
	if w.branch == "HEAD" {
		w.branch = ""
	}
	return w, nil
}

// hookFunc handles one hook event; "" means there is nothing to tell the agent.
type hookFunc func(in HookInput, w worktree, log zerolog.Logger) (string, error)

// now is replaced in tests.
var now = time.Now

// PostToolUse watches the branch after each shell command and, at a switch to
// a branch new to this session on a large context, asks the agent to suggest
// /clear.
func PostToolUse(in HookInput, w worktree, log zerolog.Logger) (string, error) {
	st, err := loadState(w.gitDir)
	if err != nil {
		return "", err
	}
	before := st.Session
	v := observe(&st, in.SessionID, w.branch, now(),
		func() Config { return loadConfig(in.Cwd) },
		func() int {
			ctx, err := ContextTokens(in.TranscriptPath)
			if err != nil {
				log.Warn().Err(err).Str("transcript", in.TranscriptPath).Msg("context unreadable; counted as 0")
			}
			return ctx
		})
	if v.Reason == ReasonSame && st.Session == before {
		return "", nil // nothing changed: no write
	}
	if err := saveState(w.gitDir, st); err != nil {
		return "", err
	}
	if v.Reason == ReasonSame || v.Reason == ReasonFirst {
		return "", nil
	}
	if p := v.Replaced; p != nil {
		log.Info().Str("session", in.SessionID).Str("from", p.From).Str("to", p.To).Int("context", p.Context).Msg(msgReplaced)
	}
	ev := log.Info().Str("session", in.SessionID).Str("from", v.From).Str("to", v.To).Str("reason", v.Reason)
	if v.Threshold > 0 {
		ev = ev.Int("context", v.Context).Int("threshold", v.Threshold)
	}
	ev.Bool("suggested", v.Reason == ReasonSuggest).Msg(msgBoundary)
	if v.Reason != ReasonSuggest {
		return "", nil
	}
	return fmt.Sprintf("Loci: the branch changed from %s to %s while the context is %dk tokens. "+
		"This is a task boundary. Tell the user they can run /clear now; Loci will hand the "+
		"git state of %s to the new session, so nothing from git is lost.", v.From, v.To, v.Context/1000, v.To), nil
}

// SessionStart hands a pending boundary over to a cleared or new session.
func SessionStart(in HookInput, w worktree, log zerolog.Logger) (string, error) {
	if in.Source != "clear" && in.Source != "startup" {
		return "", nil // resume and compact keep their context: nothing to hand over
	}
	st, err := loadState(w.gitDir)
	if err != nil || st.Pending == nil {
		return "", err
	}
	p := st.Pending
	st.Pending = nil
	if w.branch != "" {
		st.Branch = w.branch
	}
	if err := saveState(w.gitDir, st); err != nil {
		return "", err
	}
	if age := now().Sub(p.At); age > pendingTTL {
		log.Info().Str("session", in.SessionID).Str("to", p.To).Dur("age", age).Msg(msgStale)
		return "", nil
	}
	f, err := Collect(in.Cwd, "", p.From)
	if err != nil {
		return "", err
	}
	md := f.Markdown()
	log.Info().Str("session", in.SessionID).Str("source", in.Source).Str("from", p.From).Str("to", f.Branch).
		Int("context", p.Context).Int("chars", len(md)).Msg(msgHandoff)
	return md, nil
}

// runHook reads a hook payload, runs fn and writes Claude Code's hook JSON.
// It never fails the agent: errors go to .git/loci/log and nothing is printed.
func runHook(event string, fn hookFunc, stdin io.Reader, stdout io.Writer) {
	var in HookInput
	if err := json.NewDecoder(io.LimitReader(stdin, 8<<20)).Decode(&in); err != nil || in.Cwd == "" {
		return
	}
	w, err := locate(in.Cwd)
	if err != nil {
		return // not a git repository: nothing to watch
	}
	log, done := openLog(w.gitDir)
	defer done()
	msg, err := fn(in, w, log)
	if err != nil {
		log.Error().Err(err).Str("event", event).Msg(msgFailed)
		return
	}
	if msg == "" {
		return
	}
	var out hookOutput
	out.HookSpecificOutput.HookEventName = event
	out.HookSpecificOutput.AdditionalContext = msg
	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		log.Error().Err(err).Str("event", event).Msg("hook output failed")
	}
}
