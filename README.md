# Loci

> Ask how your code works, not where a name is.

English | [繁體中文](README.zh-TW.md)

**Status: closed.** Loci was measured before it was built, and it does not pay off: no
version beat plain text search on both tokens and cost in a reproducible way. It is not being
built. See [the results](doc/measurement.md#3-results). The rest of this page describes the
hypothesis that was tested.

Loci answers questions about a behaviour with the functions that carry it, and why they are
written that way: a location and a one-line note. It keeps that memory local and in step with
git.

```text
> what happens to sign-in when the session has expired?
loci  auth/session.rs:88  refresh_or_reject
      an expired token is refreshed once, then rejected (note @a1b2c3)
```

Looking up a name or a keyword is text search's job. Text search finds a function in one or
two calls, even on a large repository; Loci does not try to beat it there.

## Why

An agent that starts cold on a repository can find a name quickly, but not the reason behind
the code. It reads far more than it needs to learn why, and the next session learns it again.

Loci keeps what was learned:

- **How does it work?** A question about a behaviour, answered with the functions that carry it.
- **Why is it written that way?** A one-line note per function, left for the next agent.
- **For which revision?** The commit you have checked out, not a stale snapshot.

The benchmark tested this with a stand-in for Loci. Its one gain, notes on concept questions
handed over by a prompt hook, did not replicate; see [the results](doc/measurement.md#3-results).

## Concept

```text
  ┌──────────────────┐  ┌──────────────────┐      ┌──────────────────────────┐
  │ git hooks        │  │ command line     │      │ agent                    │
  │ (per repository) │  │ e.g. rebuild     │      │ skill + prompt hook      │
  └────────┬─────────┘  └────────┬─────────┘      └──────────────────────────┘
           │                     │                             ▲
           │ git event           │ command                     │ ask, answer
           ▼                     ▼                             ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ Loci                                                                         │
│ ┌────────────────────────────────────────┐  ┌──────────────────────────────┐ │
│ │ event receiver                         │  │ loci CLI                     │ │
│ │ takes git events and commands,         │  │ find, show, search, outline; │ │
│ │ returns at once                        │  │ suggest; writes notes        │ │
│ └───────────────────┬────────────────────┘  └──────────────────────────────┘ │
│                     │                                        ▲               │
│                     ▼                                        │               │
│ ┌────────────────────────────────────────┐                   │               │
│ │ builder                                │                   │               │
│ │ parses each commit (tree-sitter),      │                   │ read, write   │
│ │ publishes it all at once               │                   │               │
│ └───────────────────┬────────────────────┘                   │               │
│                     │ publish                                │               │
│                     ▼                                        ▼               │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ data storage, per repository                                             │ │
│ │ ┌────────────────────┐  ┌────────────────────┐  ┌────────────────────┐   │ │
│ │ │ repo A             │  │ repo B             │  │ repo ...           │   │ │
│ │ │ symbols + notes,   │  │ symbols + notes,   │  │ symbols + notes,   │   │ │
│ │ │ per revision       │  │ per revision       │  │ per revision       │   │ │
│ │ └────────────────────┘  └────────────────────┘  └────────────────────┘   │ │
│ │ where and how much: global setup, overridden by each repo's .loci        │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```

- **Code memory.** Two layers: symbols that Loci parses out of the source — each function's
  name, file and location — and a concept note per function, written by the LLM. Loci never
  calls an LLM itself.
- **Notes on demand.** Notes are written for functions that are asked about or recently
  changed, not for every function up front. Gate 1 tested this, and it did not pay off.
- **Git-aware.** Git operations are the trigger. A commit builds its memory, a checkout
  switches to the memory of that revision, and dropping branches or commits prunes what
  nothing can reach any more.
- **A standalone service.** One personal, local service for all your projects, with a global
  setup that each repository can override through a `.loci` file.
- **Shared by agents.** A prompt hook (`UserPromptSubmit`) runs `loci suggest` and injects
  candidate functions. The `loci` CLI (`find`, `show`, `search`, `outline`) and its skill let
  the agent look further and write notes back. There is no MCP server. What one agent works
  out, the next one starts with.

## Flow

What happens from a commit to an agent's answer:

```text
┌──────────────────────────────┐
│ git commit                   │
└───────────────┬──────────────┘
                │ hook fires
                ▼
┌──────────────────────────────┐
│ build the commit's memory    │   queries meanwhile get the
│ in the background            │   last completed memory
└───────────────┬──────────────┘
                │ build complete
                ▼
┌──────────────────────────────┐
│ publish all at once          │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ prompt submitted: the hook   │
│ runs loci suggest            │
└───────────────┬──────────────┘
                │ candidates injected
                ▼
┌──────────────────────────────┐
│ agent asks the loci CLI      │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ answer: location, note,      │
│ and the commit it describes  │
└───────────────┬──────────────┘
                │ note missing or stale
                ▼
┌──────────────────────────────┐
│ agent reads the code, writes │
│ the note through the CLI     │
└──────────────────────────────┘
```

The other triggers:

```text
checkout / switch ─────────> switch to that revision's memory
branch or commit dropped ──> prune what nothing can reach
hook missed ───────────────> rebuild on start, by command line or by skill
```

The full rules are in the [specification](doc/spec.md).

## Principles

- **Personal and local.** Your code and its memory stay on your machine.
- **Never in the way.** A git operation is never blocked or failed by Loci.
- **Hook-driven.** Git operations build the memory at once; nothing waits for a query.
- **All or nothing.** A commit's memory is visible only once it is completely built.
- **Revision-true.** An answer always describes the code of the commit it names.
- **Small answers.** A reply is a location and a line or two, not a file dump.

## Roadmap

- **Feature maps.** Answer "which functions take part in X". Dropped: in gate 1 it did not beat
  text search.

## Documentation

- [Specification](doc/spec.md) — how the memory behaves: functions and notes, builds,
  recovery, pruning and configuration.
- [Measurement](doc/measurement.md) — how speed and token usage are measured, and the results
  that closed the project.

## License

[MIT](LICENSE)
