// Package loci clears an agent's context at git boundaries and hands the git
// state of the new task to the next session.
package loci

import (
	"errors"
	"fmt"
	"io"

	"github.com/alecthomas/kong"
)

// Version is set at build time with -ldflags "-X github.com/cmj0121/loci/internal/loci.Version=…".
var Version = "0.0.0-dev"

var errNotImplemented = errors.New("not implemented yet")

// env carries the process streams into command Run methods, so tests can swap them.
type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

// CLI is the whole command tree.
type CLI struct {
	Hook    HookCmd          `cmd:"" help:"Agent hook entry points; hook JSON on stdin."`
	Handoff HandoffCmd       `cmd:"" help:"Print the git handoff for the current branch."`
	Install InstallCmd       `cmd:"" help:"Add Loci's hooks to .claude/settings.local.json."`
	Stats   StatsCmd         `cmd:"" help:"Summarise what the hooks did in this repository."`
	Replay  ReplayCmd        `cmd:"" help:"Replay past agent sessions under Loci's rule and estimate the saving."`
	Version kong.VersionFlag `help:"Print the version."`
}

// HookCmd groups the Claude Code hooks. They always exit 0 and print nothing on error.
type HookCmd struct {
	PostToolUse  PostToolUseCmd  `cmd:"" name:"post-tool-use" help:"PostToolUse: suggest /clear at a switch to a new branch."`
	SessionStart SessionStartCmd `cmd:"" name:"session-start" help:"SessionStart: inject a pending handoff."`
}

type PostToolUseCmd struct{}

func (PostToolUseCmd) Run(e *env) error {
	runHook("PostToolUse", PostToolUse, e.stdin, e.stdout)
	return nil
}

type SessionStartCmd struct{}

func (SessionStartCmd) Run(e *env) error { return errNotImplemented }

type HandoffCmd struct {
	Dir    string `short:"C" default:"." help:"Repository directory."`
	Base   string `help:"Base ref (default: loci.base, origin/main, origin/master, origin/HEAD, main, master)."`
	Format string `enum:"md,json" default:"md" help:"Output format: md or json."`
}

func (c *HandoffCmd) Run(e *env) error {
	f, err := Collect(c.Dir, c.Base, "")
	if err != nil {
		return err
	}
	out := f.Markdown()
	if c.Format == "json" {
		out = f.JSON()
	}
	_, err = fmt.Fprint(e.stdout, out)
	return err
}

type InstallCmd struct {
	Dir       string `short:"C" default:"." help:"Repository directory."`
	Uninstall bool   `help:"Remove Loci's hooks instead."`
}

func (c *InstallCmd) Run(e *env) error { return errNotImplemented }

type StatsCmd struct {
	Dir  string `short:"C" default:"." help:"Repository directory."`
	JSON bool   `name:"json" help:"Print JSON instead of a table."`
}

func (c *StatsCmd) Run(e *env) error { return errNotImplemented }

type ReplayCmd struct {
	Dir  string `short:"C" default:"." help:"Repository whose agent sessions are replayed."`
	JSON bool   `name:"json" help:"Print JSON instead of a table."`
}

func (c *ReplayCmd) Run(e *env) error { return errNotImplemented }

// exit carries kong's exit code (from --help, --version) out of Parse.
type exit int

// Run is the whole CLI; it returns the process exit code: 0 ok, 1 failure, 2 usage.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("loci"),
		kong.Description("Hand an agent's session over at git boundaries."),
		kong.Writers(stdout, stderr),
		kong.Vars{"version": Version},
		kong.Exit(func(c int) { panic(exit(c)) }),
		kong.UsageOnError(),
	)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "loci:", err)
		return 1
	}
	defer func() {
		if r := recover(); r != nil {
			c, ok := r.(exit)
			if !ok {
				panic(r)
			}
			code = int(c)
		}
	}()
	ctx, err := parser.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "loci:", err)
		if _, ok := errors.AsType[*kong.ParseError](err); ok {
			return 2
		}
		return 1
	}
	if err := ctx.Run(&env{stdin: stdin, stdout: stdout, stderr: stderr}); err != nil {
		_, _ = fmt.Fprintln(stderr, "loci:", err)
		return 1
	}
	return 0
}
