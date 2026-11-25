package loci

import (
	"fmt"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

// step observes branch br in session s with the given context.
func see(st *State, s, br string, ctx int) Verdict {
	return observe(st, s, br, t0, defaultConfig, func() int { return ctx })
}

func TestObserveSequence(t *testing.T) {
	var st State
	for i, c := range []struct {
		session, branch string
		ctx             int
		want            string
	}{
		{"s1", "feat/gh-1-a", 300_000, ReasonFirst},
		{"s1", "feat/gh-1-a", 300_000, ReasonSame},
		{"s1", "fix/gh-2-b", 50_000, ReasonSmall},
		{"s1", "main", 300_000, ReasonLongLived},
		{"s1", "feat/gh-1-a", 300_000, ReasonRevisit},
		{"s1", "feat/gh-3-c", 300_000, ReasonSuggest},
		{"s2", "feat/gh-1-a", 300_000, ReasonSuggest}, // a new session forgets what it visited
	} {
		v := see(&st, c.session, c.branch, c.ctx)
		if v.Reason != c.want {
			t.Fatalf("step %d %s: reason %q, want %q", i, c.branch, v.Reason, c.want)
		}
	}
	if st.Pending == nil || st.Pending.From != "feat/gh-3-c" || st.Pending.To != "feat/gh-1-a" {
		t.Errorf("pending = %+v", st.Pending)
	}
}

func TestObserveReadsNothingUnlessItMust(t *testing.T) {
	st := State{Branch: "feat/a", Sessions: map[string]*Session{"s": {Visited: []string{"feat/b"}}}}
	calls := 0
	cfg := func() Config { calls++; return defaultConfig() }
	ctx := func() int { calls++; return 1 << 20 }
	for _, br := range []string{"feat/a", "", "feat/b"} { // same, detached, revisit
		observe(&st, "s", br, t0, cfg, ctx)
	}
	if calls != 0 {
		t.Errorf("config or context read %d times", calls)
	}
	observe(&st, "s", "main", t0, cfg, ctx)
	if calls != 1 {
		t.Errorf("a long-lived switch read %d things, want only the config", calls)
	}
}

func TestObserveReplacesAnUnusedSuggestion(t *testing.T) {
	st := State{Branch: "feat/a"}
	see(&st, "s", "feat/b", 200_000)
	v := see(&st, "s", "feat/c", 250_000)
	if v.Reason != ReasonSuggest || v.Replaced == nil || v.Replaced.To != "feat/b" {
		t.Fatalf("verdict %+v", v)
	}
	if st.Pending.To != "feat/c" || st.Pending.Context != 250_000 || !st.Pending.At.Equal(t0) {
		t.Errorf("pending = %+v", st.Pending)
	}
}

func TestObserveThresholdIsInclusive(t *testing.T) {
	st := State{Branch: "feat/a"}
	if v := see(&st, "", "feat/b", DefaultThreshold); v.Reason != ReasonSuggest {
		t.Errorf("at the threshold: %q", v.Reason)
	}
}

func TestLongLived(t *testing.T) {
	c := defaultConfig()
	c.Skip = append(c.Skip, "hotfix/*")
	for br, want := range map[string]bool{
		"main": true, "release/1.2": true, "sova-production": true, "hotfix/x": true,
		"feat/main-menu": false, "fix/release-notes": false,
	} {
		if got := c.longLived(br); got != want {
			t.Errorf("longLived(%q) = %v", br, got)
		}
	}
}

func TestRememberKeepsOrderAndBound(t *testing.T) {
	v := remember([]string{"a", "b"}, "a")
	if fmt.Sprint(v) != "[b a]" {
		t.Errorf("remember = %v", v)
	}
	for i := range maxVisited + 10 {
		v = remember(v, fmt.Sprint(i))
	}
	if len(v) != maxVisited || v[len(v)-1] != fmt.Sprint(maxVisited+9) {
		t.Errorf("len %d last %s", len(v), v[len(v)-1])
	}
}

func TestObserveKeepsSessionsApart(t *testing.T) {
	var st State
	see(&st, "s1", "feat/a", 300_000)
	see(&st, "s1", "feat/b", 300_000) // s1 leaves feat/a
	if v := see(&st, "s2", "feat/b", 300_000); v.Reason != ReasonSame {
		t.Fatalf("s2 on the shared HEAD: %q", v.Reason)
	}
	if v := see(&st, "s1", "feat/a", 300_000); v.Reason != ReasonRevisit {
		t.Errorf("s1 back to feat/a after s2 ran: %q, want revisit", v.Reason)
	}
	if v := see(&st, "s2", "feat/c", 300_000); v.Reason != ReasonSuggest || st.Pending.Session != "s2" {
		t.Errorf("s2 to a new branch: %q, pending %+v", v.Reason, st.Pending)
	}
}

func TestObserveTouchesSeenAtMostEveryMinute(t *testing.T) {
	var st State
	at := t0
	step := func(d time.Duration) bool {
		at = at.Add(d)
		return observe(&st, "s", "feat/a", at, defaultConfig, func() int { return 0 }).touched
	}
	if !step(0) || step(10*time.Second) || !step(seenEvery) {
		t.Errorf("seen = %v", st.Sessions["s"].Seen)
	}
}

func TestStateForgetsOldAndExtraSessions(t *testing.T) {
	var st State
	st.session("old", t0).Seen = t0
	at := t0.Add(pendingTTL + time.Hour)
	for i := range maxSessions + 3 {
		st.session(fmt.Sprint(i), at).Seen = at.Add(time.Duration(i) * time.Second)
	}
	if len(st.Sessions) != maxSessions || st.Sessions["old"] != nil || st.Sessions["0"] != nil {
		t.Errorf("sessions = %d, old %v", len(st.Sessions), st.Sessions["old"])
	}
	if st.Sessions[fmt.Sprint(maxSessions+2)] == nil {
		t.Error("the newest session was dropped")
	}
}
