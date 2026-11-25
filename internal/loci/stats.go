package loci

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Stats summarises the hooks' logs of one repository, over all its worktrees.
type Stats struct {
	Repo      string         `json:"repo"`
	Logs      int            `json:"logs"`
	First     *time.Time     `json:"first,omitempty"`
	Last      *time.Time     `json:"last,omitempty"`
	Switches  int            `json:"switches"`  // branch switches the rule judged
	Reasons   map[string]int `json:"reasons"`   // switches by verdict
	Suggested int            `json:"suggested"` // suggestions to run /clear
	Handoffs  int            `json:"handoffs"`  // handoffs injected into a new session
	Replaced  int            `json:"replaced"`  // suggestions nobody acted on before the next one
	Stale     int            `json:"stale"`     // suggestions picked up too late
	Waiting   int            `json:"waiting"`   // suggestions still pending now
	Reminded  int            `json:"reminded"`  // reminders at a turn's end
	Gated     int            `json:"gated"`     // prompts held once for a waiting handoff
	Cleared   int            `json:"cleared"`   // context tokens the handoffs replaced
	Subagents int            `json:"subagents"` // subagents that stopped
	Delegated int            `json:"delegated"` // context tokens subagents kept out of the main session
	Failures  int            `json:"failures"`  // hook errors

	agents map[string]int // a resumed subagent stops again: keep its largest context
}

// Adoption is the share of suggestions that ended in a handoff, 0 to 1.
func (s Stats) Adoption() float64 {
	if s.Suggested == 0 {
		return 0
	}
	return float64(s.Handoffs) / float64(s.Suggested)
}

// logEntry is the part of a log line stats reads.
type logEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Reason  string    `json:"reason"`
	Context int       `json:"context"`
	Agent   string    `json:"agent"`
	Kept    bool      `json:"kept"` // a handoff copied to a session beside the live one
}

// Gather reads every worktree's log of the repository at dir; since > 0 keeps
// only the entries newer than now - since.
func Gather(dir string, since time.Duration, now time.Time) (Stats, error) {
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Stats{}, err
	}
	// The repository is named after the main worktree, from any of its worktrees.
	s := Stats{Repo: filepath.Base(filepath.Dir(common)), Reasons: map[string]int{}, agents: map[string]int{}}
	gitDirs, _ := filepath.Glob(filepath.Join(common, "worktrees", "*"))
	gitDirs = append([]string{common}, gitDirs...)
	var cut time.Time
	if since > 0 {
		cut = now.Add(-since)
	}
	for _, gd := range gitDirs {
		if st, err := loadState(gd); err == nil && st.Pending != nil && st.Pending.At.After(cut) {
			s.Waiting++
		}
		f, err := os.Open(logPath(gd))
		if err != nil {
			continue
		}
		s.Logs++
		err = s.read(f, cut)
		_ = f.Close()
		if err != nil {
			return s, fmt.Errorf("%s: %w", logPath(gd), err)
		}
	}
	s.Subagents = len(s.agents)
	for _, n := range s.agents {
		s.Delegated += n
	}
	return s, nil
}

func (s *Stats) read(r io.Reader, cut time.Time) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e logEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Time.Before(cut) {
			continue // a line from another version, or outside the window
		}
		switch e.Message {
		case msgBoundary:
			s.Switches++
			s.Reasons[e.Reason]++
			if e.Reason == ReasonSuggest {
				s.Suggested++
			}
		case msgHandoff:
			if e.Kept {
				continue // a copy: the handoff still waits for its own /clear
			}
			s.Handoffs++
			s.Cleared += e.Context
		case msgReplaced:
			s.Replaced++
		case msgStale:
			s.Stale++
		case msgReminded:
			s.Reminded++
		case msgGated:
			s.Gated++
		case msgSubagent:
			if e.Agent == "" {
				continue
			}
			s.agents[e.Agent] = max(s.agents[e.Agent], e.Context)
		case msgFailed:
			s.Failures++
		default:
			continue
		}
		if s.First == nil || e.Time.Before(*s.First) {
			t := e.Time
			s.First = &t
		}
		if s.Last == nil || e.Time.After(*s.Last) {
			t := e.Time
			s.Last = &t
		}
	}
	return sc.Err()
}

// Table renders the stats for a terminal.
func (s Stats) Table() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Loci stats for %s", s.Repo)
	if s.First != nil {
		fmt.Fprintf(&b, " (%s to %s)", s.First.Local().Format("2006-01-02"), s.Last.Local().Format("2006-01-02"))
	}
	b.WriteString("\n\n")
	row := func(label string, n int, note string) {
		fmt.Fprintf(&b, "%-22s %8d", label, n)
		if note != "" {
			b.WriteString("  " + note)
		}
		b.WriteString("\n")
	}
	row("branch switches", s.Switches, "")
	for _, r := range []string{ReasonSuggest, ReasonRevisit, ReasonLongLived, ReasonSmall} {
		row("  "+r, s.Reasons[r], "")
	}
	row("suggestions", s.Suggested, "")
	row("  handoffs", s.Handoffs, fmt.Sprintf("%.0f%% adopted", 100*s.Adoption()))
	row("  replaced", s.Replaced, "the next switch came first")
	row("  stale", s.Stale, "picked up after 24h")
	row("  waiting", s.Waiting, "pending now")
	row("  reminded", s.Reminded, "at a turn's end")
	row("  gated", s.Gated, "a prompt held once (loci.nudge gate)")
	note := ""
	if s.Handoffs > 0 {
		note = fmt.Sprintf("%s on average", tokens(s.Cleared/s.Handoffs))
	}
	fmt.Fprintf(&b, "%-22s %8s  %s\n", "context cleared", tokens(s.Cleared), note)
	fmt.Fprintf(&b, "%-22s %8s  %d subagents kept it out of the main session\n", "context delegated", tokens(s.Delegated), s.Subagents)
	row("hook failures", s.Failures, "see .git/loci/log")
	if s.Logs == 0 {
		b.WriteString("\nNo log yet: run `loci install`, then work in an agent session.\n")
	}
	return b.String()
}

// tokens formats a token count as 950, 12k or 1.2M.
func tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
	case n >= 1000:
		return strconv.Itoa(n/1000) + "k"
	}
	return strconv.Itoa(n)
}

// window is a --since value: a Go duration or a number of days, such as 7d.
type window time.Duration

func (w *window) UnmarshalText(b []byte) error {
	s := string(b)
	if d, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(d)
		if err != nil || n < 0 {
			return fmt.Errorf("bad number of days %q", s)
		}
		*w = window(time.Duration(n) * 24 * time.Hour)
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return fmt.Errorf("bad duration %q: use 7d or 36h", s)
	}
	*w = window(d)
	return nil
}
