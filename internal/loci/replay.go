package loci

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Price weights relative to one input token, as on Anthropic's price list:
// a cache write costs 1.25×, a cache read 0.1×, an output token 5×.
const (
	weightInput      = 1.0
	weightCacheWrite = 1.25
	weightCacheRead  = 0.1
	weightOutput     = 5.0
)

// ReplayOptions are the knobs of a replay.
type ReplayOptions struct {
	Projects      []string // Claude Code project directories holding the session transcripts
	Repo          string   // the repository's top directory: only sessions working in it count
	Config        Config   // the rule's settings
	HandoffTokens int      // the size of each injected handoff
}

// Spend is what a set of turns cost, now and under Loci.
type Spend struct {
	Tokens     float64 `json:"tokens"`
	LociTokens float64 `json:"loci_tokens"`
	Cost       float64 `json:"cost_units"` // input-token equivalents, see the weights
	LociCost   float64 `json:"loci_cost_units"`
}

func (s *Spend) add(o Spend) {
	s.Tokens += o.Tokens
	s.LociTokens += o.LociTokens
	s.Cost += o.Cost
	s.LociCost += o.LociCost
}

// Fire is one suggestion the rule would have made.
type Fire struct {
	Session string    `json:"session"`
	At      time.Time `json:"at"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Context int       `json:"context"`
}

// ReplayResult is the outcome of replaying sessions under the rule.
type ReplayResult struct {
	Sessions      int               `json:"sessions"`
	Turns         int               `json:"turns"`
	Switches      int               `json:"switches"`
	Reasons       map[string]int    `json:"reasons"`
	Fires         []Fire            `json:"fires"`
	ByPurpose     map[string]*Spend `json:"by_purpose"` // feature, bugfix, other: by the turn's branch
	Total         Spend             `json:"total"`
	Threshold     int               `json:"threshold"`
	HandoffTokens int               `json:"handoff_tokens"`
}

// turn is one main-chain model call; step is a turn or a finished shell command, in order.
type turn struct {
	usage  usage
	branch string
}

type step struct {
	turn        int    // index into turns, or -1 for a shell command
	cwd, branch string // a shell command's directory and the branch after it
	at          time.Time
}

type session struct {
	id    string
	start time.Time
	turns []turn
	steps []step
	cwd   string
}

// sessionRecord is the part of a transcript line replay reads.
type sessionRecord struct {
	Type        string    `json:"type"`
	IsSidechain bool      `json:"isSidechain"`
	Cwd         string    `json:"cwd"`
	GitBranch   string    `json:"gitBranch"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		ID      string          `json:"id"`
		Usage   *usage          `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
}

// readSession parses one transcript: its turns, counted once per message,
// and its finished shell commands with the branch git reported after them.
func readSession(path string) (session, error) {
	f, err := os.Open(path)
	if err != nil {
		return session{}, err
	}
	defer func() { _ = f.Close() }()
	s := session{id: strings.TrimSuffix(filepath.Base(path), ".jsonl")}
	seen := map[string]bool{}
	bash := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r sessionRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.IsSidechain {
			continue
		}
		if s.start.IsZero() && !r.Timestamp.IsZero() {
			s.start = r.Timestamp
		}
		if s.cwd == "" {
			s.cwd = r.Cwd
		}
		var blocks []contentBlock
		_ = json.Unmarshal(r.Message.Content, &blocks) // a string content has no blocks
		switch r.Type {
		case "assistant":
			for _, b := range blocks {
				if b.Type == "tool_use" && b.Name == "Bash" {
					bash[b.ID] = true
				}
			}
			if u := r.Message.Usage; u != nil && !seen[r.Message.ID] {
				seen[r.Message.ID] = true
				s.turns = append(s.turns, turn{usage: *u, branch: r.GitBranch})
				s.steps = append(s.steps, step{turn: len(s.turns) - 1, at: r.Timestamp})
			}
		case "user":
			for _, b := range blocks {
				if b.Type == "tool_result" && bash[b.ToolUseID] {
					s.steps = append(s.steps, step{turn: -1, cwd: r.Cwd, branch: r.GitBranch, at: r.Timestamp})
				}
			}
		}
	}
	return s, sc.Err()
}

// purpose sorts a branch into feature, bugfix or other by its prefix.
func purpose(branch string) string {
	b := strings.ToLower(branch)
	switch {
	case strings.HasPrefix(b, "feat"):
		return "feature"
	case strings.HasPrefix(b, "fix"), strings.HasPrefix(b, "bug"), strings.HasPrefix(b, "hotfix"):
		return "bugfix"
	}
	return "other"
}

var agentWorktree = regexp.MustCompile(`^(.*?/\.claude/worktrees/[^/]+)`)

// worktreeOf maps a shell command's directory to the worktree whose state it
// shares: an agent worktree under .claude/worktrees, else the repository.
func worktreeOf(repo, cwd string) string {
	if m := agentWorktree.FindStringSubmatch(cwd); m != nil {
		return m[1]
	}
	if within(repo, cwd) {
		return repo
	}
	return cwd
}

// within reports whether path is dir or below it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Replay runs every session of the repository, oldest first, through the
// rule, as if the user ran /clear at each suggestion, and adds up the cost of
// every turn now and under Loci.
//
// After a suggestion the context of each later turn is modelled as the
// session's own floor (its smallest context among the first three turns) plus
// the handoff plus what the session grew since the suggestion. The first turn
// after a handoff writes that new prefix to the cache. A session that
// compacted on its own ends the model.
func Replay(o ReplayOptions) (ReplayResult, error) {
	res := ReplayResult{Reasons: map[string]int{}, ByPurpose: map[string]*Spend{}, Fires: []Fire{},
		Threshold: o.Config.Threshold, HandoffTokens: o.HandoffTokens}
	var sessions []session
	for _, dir := range o.Projects {
		paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if err != nil {
			return res, err
		}
		for _, p := range paths {
			s, err := readSession(p)
			if err != nil {
				return res, fmt.Errorf("%s: %w", p, err)
			}
			if len(s.turns) > 0 && (o.Repo == "" || within(o.Repo, s.cwd)) {
				sessions = append(sessions, s)
			}
		}
	}
	slices.SortFunc(sessions, func(a, b session) int { return a.start.Compare(b.start) })
	states := map[string]*State{} // per worktree, across sessions, like .git/loci/state.json
	for _, s := range sessions {
		res.Sessions++
		res.Turns += len(s.turns)
		replaySession(s, o, states, &res)
	}
	for _, p := range res.ByPurpose {
		res.Total.add(*p)
	}
	return res, nil
}

func replaySession(s session, o ReplayOptions, states map[string]*State, res *ReplayResult) {
	ctx := make([]int, len(s.turns))
	for i, t := range s.turns {
		ctx[i] = t.usage.prompt()
	}
	floor := slices.Min(ctx[:min(3, len(ctx))])
	gen := 0                      // a cleared session gets a new id, as in Claude Code
	last, anchor := 0, -1         // the latest turn; the context at the suggestion, -1 for none
	carried, pendingWrite := 0, 0 // the new session's context; its first cache write
	modelled := func(c int) int {
		if anchor < 0 {
			return c
		}
		return carried + c - anchor
	}
	for _, st := range s.steps {
		if st.turn < 0 {
			if st.branch == "" || st.branch == "HEAD" || st.cwd == "" {
				continue
			}
			key := worktreeOf(o.Repo, st.cwd)
			if states[key] == nil {
				states[key] = &State{}
			}
			cur := modelled(ctx[last])
			v := observe(states[key], fmt.Sprintf("%s-%d", s.id, gen), st.branch, st.at,
				func() Config { return o.Config }, func() int { return cur })
			if v.Reason == ReasonSame || v.Reason == ReasonFirst {
				continue
			}
			res.Switches++
			res.Reasons[v.Reason]++
			if v.Reason != ReasonSuggest {
				continue
			}
			res.Fires = append(res.Fires, Fire{Session: s.id, At: st.at, From: v.From, To: v.To, Context: cur})
			states[key].Pending = nil // the user runs /clear and the handoff is injected
			gen++
			anchor, carried = ctx[last], floor+o.HandoffTokens
			pendingWrite = carried
			continue
		}
		last = st.turn
		t := s.turns[st.turn]
		c := ctx[st.turn]
		if anchor >= 0 && c < anchor {
			anchor = -1 // the session compacted on its own: nothing left to model
		}
		u := t.usage
		read := max(0, u.CacheRead-(c-modelled(c)))
		write := u.CacheCreate + pendingWrite
		pendingWrite = 0
		p := res.ByPurpose[purpose(t.branch)]
		if p == nil {
			p = &Spend{}
			res.ByPurpose[purpose(t.branch)] = p
		}
		out := float64(u.Output) * weightOutput
		p.add(Spend{
			Tokens:     float64(u.Input + u.CacheCreate + u.CacheRead),
			LociTokens: float64(u.Input + write + read),
			Cost:       float64(u.Input)*weightInput + float64(u.CacheCreate)*weightCacheWrite + float64(u.CacheRead)*weightCacheRead + out,
			LociCost:   float64(u.Input)*weightInput + float64(write)*weightCacheWrite + float64(read)*weightCacheRead + out,
		})
	}
}

// ProjectDirs finds the Claude Code project directories for the repository at
// top: the one named after it and those of sessions started below it, such as
// agent worktrees. Sessions are filtered by their directory later.
func ProjectDirs(root, top string) []string {
	name := strings.NewReplacer("/", "-", ".", "-").Replace(top)
	all, _ := filepath.Glob(filepath.Join(root, name+"*"))
	var out []string
	for _, d := range all {
		if b := filepath.Base(d); b == name || strings.HasPrefix(b, name+"-") {
			out = append(out, d)
		}
	}
	return out
}

// Table renders a replay for a terminal.
func (r ReplayResult) Table(top int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Replayed %d sessions, %d model turns.\n", r.Sessions, r.Turns)
	fmt.Fprintf(&b, "Rule: a switch to a new short-lived branch at %s context or more; a %d-token handoff; /clear at every suggestion.\n\n",
		tokens(r.Threshold), r.HandoffTokens)
	fmt.Fprintf(&b, "Branch switches judged: %d (suggest %d, revisit %d, long-lived %d, small %d)\n\n", r.Switches,
		r.Reasons[ReasonSuggest], r.Reasons[ReasonRevisit], r.Reasons[ReasonLongLived], r.Reasons[ReasonSmall])
	fmt.Fprintf(&b, "%-8s  %s  %s\n", "purpose", padLeft("tokens now → with Loci", 28), padLeft("cost units now → with Loci", 28))
	row := func(name string, s Spend) {
		fmt.Fprintf(&b, "%-8s  %s  %s\n", name, padLeft(change(s.Tokens, s.LociTokens), 28), padLeft(change(s.Cost, s.LociCost), 28))
	}
	for _, p := range []string{"feature", "bugfix", "other"} {
		if s := r.ByPurpose[p]; s != nil {
			row(p, *s)
		}
	}
	row("total", r.Total)
	b.WriteString("\nCost units are input-token equivalents: cache write 1.25×, cache read 0.1×, output 5×.\n")
	if len(r.Fires) > 0 && top > 0 {
		fires := slices.Clone(r.Fires)
		slices.SortFunc(fires, func(a, b Fire) int { return b.Context - a.Context })
		fmt.Fprintf(&b, "\nLargest suggestions:\n")
		for _, f := range fires[:min(top, len(fires))] {
			fmt.Fprintf(&b, "  %5s  %s → %s  (%s, %s)\n", tokens(f.Context), f.From, f.To, f.Session[:min(8, len(f.Session))], f.At.Local().Format("2006-01-02"))
		}
	}
	return b.String()
}

// padLeft right-aligns s in n columns, counting runes, not bytes.
func padLeft(s string, n int) string {
	return strings.Repeat(" ", max(0, n-utf8.RuneCountInString(s))) + s
}

// change renders "1119.4M → 685.2M (-39%)".
func change(now, loci float64) string {
	pct := 0.0
	if now > 0 {
		pct = 100 * (loci/now - 1)
	}
	return fmt.Sprintf("%s → %s (%+.0f%%)", tokens(int(now)), tokens(int(loci)), pct)
}
