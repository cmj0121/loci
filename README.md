# Loci

> Start each task with a clean context, and keep what git knows.

English | [繁體中文](README.zh-TW.md)

An agent that works through several tasks in one session carries every earlier task in its
context. Most of what a long session pays for is that carried context, read again from the cache
on every turn. Loci notices when a new task starts, a switch to a new git branch, and suggests
`/clear`. The next session gets a short handoff built from git, so nothing git knows is lost.

```text
Loci: the branch changed from feat/gh-8380-scan-widget-elapsed to fix/gh-8388-crawler-qa-fixes
while the context is 297k tokens. This is a task boundary. Tell the user they can run /clear
now; Loci will hand the git state of fix/gh-8388-crawler-qa-fixes to the new session, so nothing
from git is lost.
```

On the task above, an agent run with Loci cost **$6.20 to $6.55 instead of $10.88 to $15.29**
without it, and passed the same tests. See [the measurement](doc/handoff.md).

## Why

On one product repository, 97.8% of the tokens an agent used were cache reads: the carried
context, 125k to 270k tokens, read again on every turn. Editing code was under 4%. A cache read
is cheap per token, but it is paid on every turn, so it grows with how much earlier work the
session still holds.

A git branch is a clear task boundary that the agent itself creates. Loci uses it:

- **When:** a switch to a branch this session has not left before, on a large context.
- **What:** a handoff of under 2k tokens, built from git alone. No model call, nothing guessed.
- **How:** Claude Code hooks. Other agents can print the same handoff with `loci handoff`.

## How it works

```text
┌────────────┐  after a shell command  ┌──────────────────────────────┐
│ Claude     │ ─────────────────────>  │ loci hook post-tool-use      │
│ Code       │                         │ new branch, context ≥ 100k?  │
│            │ <─────────────────────  │ yes: suggest /clear,         │
│            │  "you can run /clear"   │ record a pending handoff     │
│            │                         └──────────────┬───────────────┘
│            │                                        │ .git/loci/state.json
│            │  session starts after /clear           ▼
│            │ ─────────────────────>  ┌──────────────────────────────┐
│            │ <─────────────────────  │ loci hook session-start      │
│            │  the git handoff        │ git: branch, issue, base,    │
└────────────┘                         │ commits, changes, status     │
                                       └──────────────────────────────┘
```

The rule, applied after each shell command:

| Switch                                               | Verdict                  |
| ---------------------------------------------------- | ------------------------ |
| to a branch this session already left                | revisit: mid-task, quiet |
| to `main`, a release branch, the base or a skip glob | long-lived: quiet        |
| to a new branch, context under the threshold         | small: quiet             |
| to a new branch, context at or over the threshold    | suggest `/clear`         |

The handoff names the repository, the branch and its issue, the previous branch, the base and
merge-base, the upstream, the commits and changed files since the base, and what is uncommitted.
It also tells the new session that the issue's text and decisions are not in its context.

Loci never runs `/clear` itself: you decide. A handoff waits for the next cleared or new session
for 24 hours. Hooks never fail the agent: errors go to `.git/loci/log`.

## Install

Loci is one Go binary. Install the prebuilt one with Homebrew on macOS or Linux:

```bash
brew install cmj0121/tap/loci
loci install -C ~/src/my-repo
```

Or build it from source, which writes `bin/loci`:

```bash
make build
```

The [releases](https://github.com/cmj0121/loci/releases) page also has the binaries for linux and
darwin on amd64 and arm64.

`loci install` adds three hooks and a status line to the repository's `.claude/settings.local.json`,
a file that is not shared with the team. It keeps your other settings, and runs your own status
line first, so the line ends in Loci's state:

```text
<your status line> · loci working · 1.2M/340k saved · /clear ready · 1 failed
```

`saved` has two fields, main/subagents. The first is the context tokens the handoffs cleared, a
floor. The second is the context subagents ended with, which stayed out of the main session; it is
what delegating saved, not a handoff, and a resumed subagent counts once. A subagent's own tool calls
never trigger a /clear suggestion: it cannot clear, and the main session judges a branch it switched
at its next command. `/clear ready` shows while a handoff
waits, and `failed` counts hook errors in the last 24 hours. `loci install --uninstall` removes the
hooks and restores your status line.

## Commands

| Command                            | What it does                                                     |
| ---------------------------------- | ---------------------------------------------------------------- |
| `loci handoff [--format md\|json]` | Print the git handoff for the current branch, for any agent      |
| `loci stats [--since 7d]`          | Count switches, suggestions, handoffs, and how many were adopted |
| `loci replay`                      | Replay this repository's past sessions under the rule; estimate  |
| `loci statusline [--then CMD]`     | Claude Code status line: run `CMD`, then append Loci's state     |
| `loci install [--uninstall]`       | Add or remove the Claude Code hooks and status line              |
| `loci hook post-tool-use`          | The PostToolUse hook (JSON on stdin)                             |
| `loci hook session-start`          | The SessionStart hook (JSON on stdin)                            |
| `loci hook subagent-stop`          | The SubagentStop hook (JSON on stdin)                            |

Every command takes `-C <dir>`; `stats` and `replay` take `--json`.

`loci replay` reads the repository's transcripts under `~/.claude/projects` and shows what the
rule would have saved, before you install anything:

```text
purpose         tokens now → with Loci    cost units now → with Loci
feature         509.5M → 280.1M (-45%)          61.2M → 38.7M (-37%)
bugfix          158.5M → 108.5M (-32%)          19.8M → 15.0M (-24%)
total           782.3M → 482.7M (-38%)          96.8M → 67.7M (-30%)
```

`loci stats` tells, after a week of real use, whether the suggestions were acted on.

## Settings

All settings live in git config, per repository or global.

| Key              | Default                                       | Meaning                                       |
| ---------------- | --------------------------------------------- | --------------------------------------------- |
| `loci.threshold` | `100000`                                      | Context tokens at which a switch is a handoff |
| `loci.skip`      | `main`, `master`, `develop`, `release/*`, ... | More long-lived branch globs (`--add`)        |
| `loci.base`      | `origin/main`, `origin/master`, ...           | The base the handoff compares with            |

## Limits

- It helps in proportion to the context at the switch: about 10% at 80k, about half at 300k.
- What was decided in conversation, not in git or the tracker, does not survive `/clear`.
- It reads the Claude Code transcript format, which is internal and may change; when it cannot
  read the context, it counts it as 0 and stays quiet.

## History

Loci began as a local code memory for agents. That hypothesis was measured and closed; its
[measurement](doc/measurement.md), [survey](doc/survey.md) and [spec](doc/spec.md) are kept as a
record.

## License

See [LICENSE](LICENSE).
