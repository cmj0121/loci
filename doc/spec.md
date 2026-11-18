# Loci specification

English | [繁體中文](spec.zh-TW.md)

This document specifies how Loci behaves. It says what the memory holds and when it changes,
not how it is implemented. For the idea behind it, read the [README](../README.md).

## 1. Scope

- Loci is a personal tool. One local service serves every project on one machine.
- Loci knows committed code only. Changes that are not committed yet are the agent's own to
  keep in mind; they enter the memory when they are committed.
- Loci never calls an LLM. Understanding is written by the agent's LLM and handed to Loci.

## 2. Code memory

The memory has two layers.

| Layer   | What it holds                           | Who writes it        |
| ------- | --------------------------------------- | -------------------- |
| Symbols | each function's name, file and location | Loci, by parsing     |
| Notes   | one concept note per function           | the LLM, through MCP |

Each part has one job:

- **tree-sitter** abstracts every function to its name, its file and its location;
- **the LLM** writes one note per function, guided by the skill;
- **MCP** is how that note is written and updated.

Symbols are extracted deterministically, so they can always be rebuilt from the repository.
Notes cannot: they are the only part of the memory that is not derived from the code.

## 3. Functions through history

A note belongs to the function itself, not to its name or its place. Loci therefore follows
each function from commit to commit.

| What changed in a commit                   | What Loci does                                                 |
| ------------------------------------------ | -------------------------------------------------------------- |
| the function is renamed                    | detects the rename — same function, new name; the note stays   |
| the function moves to another file or line | keeps the note and only replaces the location                  |
| its implementation or concept changes      | marks the note as needing an update, until the LLM rewrites it |
| the function is added                      | a new function, with no note yet                               |
| the function is removed                    | the function and its note are absent from that commit on       |

A rename is recognised by the function's content staying the same while its name changes. A
function that is renamed and substantially rewritten in the same commit cannot be told from
one function removed and another added, and is treated that way.

## 4. Notes and revisions

The memory is revision-true:

- the same function can carry a different note in different commits;
- a function that does not exist in a commit has no note there;
- on any revision, a query returns the note that belongs to the code as it is there.

A note is in one of two states.

| State        | Meaning                                         | What a query returns        |
| ------------ | ----------------------------------------------- | --------------------------- |
| current      | written for this version of the function        | the note                    |
| needs update | the function changed after the note was written | the earlier note, marked so |

A note that needs an update is still returned, so the agent has the earlier concept as a lead.
The skill tells the agent what to do with the mark: read the code, then write the new note
through MCP. Writing it makes the note current for that commit; earlier commits keep the note
they had.

## 5. Building the memory

Git hooks are the trigger. Nothing is built because of a query.

| Git operation                   | What Loci does                                         |
| ------------------------------- | ------------------------------------------------------ |
| `commit`                        | builds the memory for the new commit                   |
| `checkout` / `switch`           | switches to the memory of that revision                |
| `merge` / `rebase`              | follows the new history, reusing what it already knows |
| branch deleted, commits dropped | prunes what no revision can reach any more (section 7) |

Rules of a build:

1. **At once.** The build starts the moment the hook fires.
2. **In the background.** The hook returns immediately. A git operation is never blocked or
   failed by Loci.
3. **Per commit.** A build covers one commit, and reuses everything already known about
   content that did not change.
4. **Published all at once.** The project's memory moves to the new commit only when that
   commit's build is complete.
5. **Readers never wait and never see a half-built memory.** During a build, queries are
   answered from the last completed memory.
6. **Every answer names its commit.** An agent can always tell which commit an answer
   describes, and so whether it is reading the memory of an earlier one.

Identical content is remembered once, however many branches and commits contain it. Switching
to a revision that is already built is a lookup, not a rebuild.

## 6. Missed hooks and rebuilding

A hook can be missed: the service was not running, or the hook could not reach it. Loci never
hides the gap.

- **On start.** When the service starts, it rebuilds whatever is missing for each project's
  current commit.
- **On query.** A query on a commit that has no memory yet says so, and offers the last
  completed memory instead. It does not build.
- **On request.** The user can rebuild at any time, in two ways: from the command line, or
  through the skill.

## 7. Pruning

When branches are deleted or commits are dropped, the memory that no remaining revision can
reach is removed. A repository's configuration can also limit how much memory is kept
(section 8).

## 8. Configuration

Settings come in two levels.

| Level  | Where                          | Applies to           |
| ------ | ------------------------------ | -------------------- |
| Global | the service's own setup        | every project        |
| Local  | a `.loci` file in a repository | that repository only |

- `.loci` can override part of the global setup or all of it. Anything it does not mention
  falls back to the global setup.
- Settings include at least where a repository's memory is stored and how much of it is kept.
- Loci only reads `.loci`. Whether to commit it is the user's choice.

## 9. Agents

Every agent working on a project reads and writes the same memory.

- **Skill.** Tells the agent when to ask Loci instead of searching the full text, how to
  write a note for a function, what to do with a note that needs an update, and how to ask
  for a rebuild.
- **MCP.** The tools the agent calls: look up where a function is, read its note, write or
  update a note.

Answers are small: a location and a line or two, not a file dump.

## Open points

Not decided yet:

- the list of MCP tools and the shape of their answers;
- the settings a `.loci` file accepts, and its format;
- what "how much is kept" counts — revisions, age or size — and what is dropped first;
- how pruning treats notes, given that notes cannot be rebuilt;
- which languages are parsed first;
- whether anything besides functions (types, methods, modules) gets a note.
