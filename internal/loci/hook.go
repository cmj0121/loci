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
	// AgentID is set when the hook fires inside a subagent; TranscriptPath is
	// still the main session's, AgentTranscriptPath (SubagentStop) the subagent's.
	AgentID             string `json:"agent_id"`
	AgentType           string `json:"agent_type"`
	AgentTranscriptPath string `json:"agent_transcript_path"`
	Prompt              string `json:"prompt"`           // UserPromptSubmit: what the user sent
	StopHookActive      bool   `json:"stop_hook_active"` // Stop: the turn already continued for a stop hook
}

type hookOutput struct {
	// SystemMessage is shown to the user by Claude Code itself, so a
	// suggestion reaches them even when the agent does not pass it on.
	SystemMessage string `json:"systemMessage,omitempty"`
	// Decision "block" with Reason holds a UserPromptSubmit prompt; Claude
	// Code shows Reason and the prompt to the user, not to the agent.
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// HookSpecificOutput is left out when there is no context: at Stop,
	// context for the agent would keep the turn going.
	HookSpecificOutput *hookSpecific `json:"hookSpecificOutput,omitempty"`
}

type hookSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
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

// reply is what a hook tells: Context to the agent, Notice to the user, and
// Block, when set, holds the user's prompt with that reason.
type reply struct{ Context, Notice, Block string }

// hookFunc handles one hook event; a zero reply means there is nothing to tell.
type hookFunc func(in HookInput, w worktree, log zerolog.Logger) (reply, error)

// now is replaced in tests.
var now = time.Now

// PostToolUse watches the branch after each shell command and, at a switch to
// a branch new to this session on a large context, asks the agent to suggest
// /clear.
func PostToolUse(in HookInput, w worktree, log zerolog.Logger) (reply, error) {
	if in.AgentID != "" {
		// A subagent cannot /clear and its context is not the main session's.
		// A branch it switched is judged at the main session's next command.
		return reply{}, nil
	}
	st, err := loadState(w.gitDir)
	if err != nil {
		return reply{}, err
	}
	v := observe(&st, in.SessionID, w.branch, now(),
		func() Config { return loadConfig(in.Cwd) },
		func() int {
			ctx, err := ContextTokens(in.TranscriptPath)
			if err != nil {
				log.Warn().Err(err).Str("transcript", in.TranscriptPath).Msg("context unreadable; counted as 0")
			}
			return ctx
		})
	if v.Reason == ReasonSame && !v.touched {
		return reply{}, nil // nothing changed: no write
	}
	if err := saveState(w.gitDir, st); err != nil {
		return reply{}, err
	}
	if v.Reason == ReasonSame || v.Reason == ReasonFirst {
		return reply{}, nil
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
		return reply{}, nil
	}
	r := reply{Context: fmt.Sprintf("Loci: the branch changed from %s to %s while the context is %dk tokens. "+
		"This is a task boundary. Tell the user they can run /clear now; Loci will hand the "+
		"git state of %s to the new session, so nothing from git is lost.", v.From, v.To, v.Context/1000, v.To)}
	switch loadNudge(in.Cwd) {
	case NudgeOff:
		return r, nil
	case NudgeAsk:
		r.Context += " Before you go on, ask the user with AskUserQuestion whether to stop here so they " +
			"can run /clear, or to keep going in this session. You cannot run /clear yourself."
	}
	r.Notice = fmt.Sprintf("Loci: new task on %s with %dk tokens of context. Run /clear to start it "+
		"fresh; the git state of %s is handed to the new session.", v.To, v.Context/1000, v.To)
	return r, nil
}

// waiting is the handoff this session left, if it still waits: a pending
// boundary of another session, or a stale one, is not this session's to press.
func waiting(st State, session string, at time.Time) *Pending {
	p := st.Pending
	if p == nil || at.Sub(p.At) > pendingTTL || (p.Session != "" && p.Session != session) {
		return nil
	}
	return p
}

// Stop reminds the user, once per handoff, at the end of the turn: the moment
// they are about to type. It never tells the agent, so the turn still ends.
func Stop(in HookInput, w worktree, log zerolog.Logger) (reply, error) {
	if in.AgentID != "" || in.StopHookActive {
		return reply{}, nil
	}
	st, err := loadState(w.gitDir)
	p := waiting(st, in.SessionID, now())
	if err != nil || p == nil || p.Reminded || loadNudge(in.Cwd) == NudgeOff {
		return reply{}, err
	}
	p.Reminded = true
	if err := saveState(w.gitDir, st); err != nil {
		return reply{}, err
	}
	log.Info().Str("session", in.SessionID).Str("to", p.To).Msg(msgReminded)
	return reply{Notice: fmt.Sprintf("Loci: a handoff of %s is waiting. Run /clear to go on with a clean "+
		"context; the git state of %s carries over.", p.To, p.To)}, nil
}

// UserPromptSubmit, at loci.nudge gate, holds the user's next prompt once while
// a handoff waits, so they choose: /clear, or send it again to stay. A slash
// command, or a turn Claude Code starts itself, always goes through.
func UserPromptSubmit(in HookInput, w worktree, log zerolog.Logger) (reply, error) {
	pr := strings.TrimSpace(in.Prompt)
	if in.AgentID != "" || pr == "" || strings.HasPrefix(pr, "/") || strings.HasPrefix(pr, "<") {
		return reply{}, nil
	}
	st, err := loadState(w.gitDir)
	p := waiting(st, in.SessionID, now())
	if err != nil || p == nil || p.Gated || loadNudge(in.Cwd) != NudgeGate {
		return reply{}, err
	}
	p.Gated = true
	if err := saveState(w.gitDir, st); err != nil {
		return reply{}, err
	}
	log.Info().Str("session", in.SessionID).Str("to", p.To).Int("context", p.Context).Msg(msgGated)
	return reply{Block: fmt.Sprintf("Loci: a handoff of %s is waiting (%s → %s at %dk tokens). Run /clear "+
		"to start with a clean context; the git state carries over. To stay in this session, send your "+
		"prompt again. (git config loci.nudge notice turns this off.)", p.To, p.From, p.To, p.Context/1000)}, nil
}

// SessionStart hands a pending boundary over to a cleared or new session. A
// new session started beside the one that crossed the boundary, while that
// one is still live, gets a copy: the handoff stays for that session's /clear.
func SessionStart(in HookInput, w worktree, log zerolog.Logger) (reply, error) {
	if in.Source != "clear" && in.Source != "startup" {
		return reply{}, nil // resume and compact keep their context: nothing to hand over
	}
	st, err := loadState(w.gitDir)
	if err != nil || st.Pending == nil {
		return reply{}, err
	}
	p := st.Pending
	at := now()
	age := at.Sub(p.At)
	keep := in.Source == "startup" && age <= pendingTTL && p.Session != in.SessionID && st.live(p.Session, at)
	if !keep {
		st.Pending = nil
		if in.Source == "clear" {
			delete(st.Sessions, p.Session) // cleared: that session id is gone
		}
	}
	if w.branch != "" {
		st.Branch = w.branch
	}
	if err := saveState(w.gitDir, st); err != nil {
		return reply{}, err
	}
	if age > pendingTTL {
		log.Info().Str("session", in.SessionID).Str("to", p.To).Dur("age", age).Msg(msgStale)
		return reply{}, nil
	}
	f, err := Collect(in.Cwd, "", p.From)
	if err != nil {
		return reply{}, err
	}
	md := f.Markdown()
	log.Info().Str("session", in.SessionID).Str("source", in.Source).Str("from", p.From).Str("to", f.Branch).
		Int("context", p.Context).Int("chars", len(md)).Bool("kept", keep).Msg(msgHandoff)
	r := reply{Context: md}
	if loadNudge(in.Cwd) != NudgeOff {
		r.Notice = fmt.Sprintf("Loci: handed the git state of %s to this session.", f.Branch)
	}
	return r, nil
}

// SubagentStop records the context a subagent ended with: tokens its work
// read that never entered the main session. It tells the agent nothing.
func SubagentStop(in HookInput, w worktree, log zerolog.Logger) (reply, error) {
	if in.AgentID == "" || in.AgentTranscriptPath == "" {
		return reply{}, nil
	}
	ctx, err := SubagentTokens(in.AgentTranscriptPath)
	if err != nil {
		log.Warn().Err(err).Str("transcript", in.AgentTranscriptPath).Msg("subagent transcript unreadable")
		return reply{}, nil
	}
	if ctx == 0 {
		return reply{}, nil
	}
	log.Info().Str("session", in.SessionID).Str("agent", in.AgentID).Str("type", in.AgentType).
		Int("context", ctx).Msg(msgSubagent)
	return reply{}, nil
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
	if msg == (reply{}) {
		return
	}
	out := hookOutput{SystemMessage: msg.Notice}
	if msg.Block != "" {
		out.Decision, out.Reason = "block", msg.Block
	}
	if msg.Context != "" {
		out.HookSpecificOutput = &hookSpecific{HookEventName: event, AdditionalContext: msg.Context}
	}
	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		log.Error().Err(err).Str("event", event).Msg("hook output failed")
	}
}
