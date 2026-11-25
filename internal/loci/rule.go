package loci

import (
	"slices"
	"time"
)

// Reasons a branch observation ends with. Only ReasonSuggest asks for a handoff.
const (
	ReasonSame      = ""           // still on the same branch
	ReasonFirst     = "first"      // first branch seen in this worktree: recorded, not judged
	ReasonRevisit   = "revisit"    // back to a branch this session already left: mid-task work
	ReasonLongLived = "long-lived" // main, a release branch, the base: a look or a rebase
	ReasonSmall     = "small"      // the context is under the threshold: not worth a handoff
	ReasonSuggest   = "suggest"    // a new task on a large context: suggest /clear
)

// Verdict is the outcome of one branch observation.
type Verdict struct {
	From, To  string
	Reason    string
	Context   int      // tokens, read only for a switch to a new, short-lived branch
	Threshold int      // the threshold the context was compared with
	Replaced  *Pending // an earlier suggestion that nobody acted on
	touched   bool     // the session's Seen moved: the state needs a write
}

// observe applies one look at the current branch to st. It is the whole rule,
// shared by the hook and by replay. config and context are called only when
// the branch changed to one that is new and short-lived, so the common case
// costs no git calls and no transcript read.
func observe(st *State, session, branch string, at time.Time, config func() Config, context func() int) Verdict {
	me := st.session(session, at)
	v := Verdict{From: st.Branch, To: branch}
	if at.Sub(me.Seen) >= seenEvery {
		me.Seen, v.touched = at, true
	}
	if branch == "" || branch == st.Branch {
		v.From = ""
		return v
	}
	st.Branch = branch
	if v.From == "" {
		v.Reason = ReasonFirst
		return v
	}
	revisit := slices.Contains(me.Visited, branch)
	me.Visited = remember(me.Visited, v.From)
	if revisit {
		v.Reason = ReasonRevisit
		return v
	}
	cfg := config()
	if cfg.longLived(branch) {
		v.Reason = ReasonLongLived
		return v
	}
	v.Context, v.Threshold = context(), cfg.Threshold
	if v.Context < cfg.Threshold {
		v.Reason = ReasonSmall
		return v
	}
	v.Reason, v.Replaced = ReasonSuggest, st.Pending
	st.Pending = &Pending{From: v.From, To: branch, Context: v.Context, At: at, Session: session}
	return v
}
