# Loci

> A local, git-aware memory of your code.

English | [繁體中文](README.zh-TW.md)

Loci keeps a memory of your code — where things are and what they mean — and keeps it in step
with git. AI coding agents ask Loci instead of searching the whole repository, so they reach
the right file and line with far fewer tokens.

## Why

An agent that starts cold on a repository spends most of its budget finding its way: it greps,
opens files, reads far more than it needs, and repeats the same search in the next session.
What it learns is thrown away when the session ends.

Loci turns that into a lookup:

- **Where is it?** Functions, answered by file and location.
- **What is it for?** A short concept note per function, left for the next agent.
- **For which revision?** The commit you have checked out, not a stale snapshot.

## Concept

```text
  ┌──────────────────┐  ┌──────────────────┐      ┌──────────────────────────┐
  │ git hooks        │  │ command line     │      │ agent                    │
  │ (per repository) │  │ e.g. rebuild     │      │ skill: when and how      │
  └────────┬─────────┘  └────────┬─────────┘      └──────────────────────────┘
           │                     │                             ▲
           │ git event           │ command                     │ ask, answer
           ▼                     ▼                             ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ Loci                                                                         │
│ ┌────────────────────────────────────────┐  ┌──────────────────────────────┐ │
│ │ event receiver                         │  │ MCP server                   │ │
│ │ takes git events and commands,         │  │ finds functions and notes,   │ │
│ │ returns at once                        │  │ writes and updates notes     │ │
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
  name, file and location — and one concept note per function, written by the LLM. Loci
  never calls an LLM itself.
- **Git-aware.** Git operations are the trigger. A commit builds its memory, a checkout
  switches to the memory of that revision, and dropping branches or commits prunes what
  nothing can reach any more.
- **A standalone service.** One personal, local service for all your projects, with a global
  setup that each repository can override through a `.loci` file.
- **Shared by agents.** A skill tells the agent when to ask Loci; an MCP server gives it the
  tools to look things up and to write notes back. What one agent works out, the next one
  starts with.

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
│ agent asks through MCP       │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ answer: location, note,      │
│ and the commit it describes  │
└───────────────┬──────────────┘
                │ note needs update
                ▼
┌──────────────────────────────┐
│ agent reads the code, writes │
│ the new note through MCP     │
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

## Documentation

- [Specification](doc/spec.md) — how the memory behaves: functions and notes, builds,
  recovery, pruning and configuration.
- [Measurement](doc/measurement.md) — how speed and token usage are measured.

## License

[MIT](LICENSE)
