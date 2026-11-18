# Measuring Loci

English | [繁體中文](measurement.zh-TW.md)

Loci makes two promises: it is fast enough to stay out of the way, and it lets an agent reach
the code with fewer tokens. This document says how each is measured. The behaviour being
measured is defined in the [specification](spec.md).

Both measurements can be run before Loci exists, with an existing index tool standing in for
it. That is the cheapest way to learn whether the idea pays off.

## 1. Speed

### What is measured

| Metric        | What it is                                    | How it is measured                             |
| ------------- | --------------------------------------------- | ---------------------------------------------- |
| Hook overhead | time a hook adds to `git commit`              | same commit with and without the hook; the gap |
| Build time    | from the hook firing to the memory published  | two timestamps recorded by Loci                |
| Stale window  | how long a query still gets the old memory    | that commit's build time, worst case           |
| Query latency | from an MCP call being sent to its answer     | hundreds of calls per tool; p50 and p95        |
| Switch time   | from `checkout` to answering the new revision | switching between two built branches           |
| Storage       | disk used by a repository's memory            | size per 1,000 files; growth per 100 commits   |

Build time is reported twice: the first full build of a repository, and the build of a single
commit.

### How it is run

- **Three repository sizes** — small, medium and large — so the numbers show how each metric
  grows.
- **Real history.** Replay the last N commits of each repository rather than synthetic
  commits, so the changes have a realistic size and shape.
- **Repeat and report spread.** Every metric is the median of many runs, with the worst case
  beside it.
- **Same machine, stated.** Results name the machine they were taken on; numbers from
  different machines are not compared.

## 2. Token usage

Token usage cannot be measured on Loci alone. It is the difference between an agent working
with Loci and the same agent working without it, on the same tasks.

### Tasks

A fixed list of tasks on a real repository. Each task has a known answer — a file and a line
range — decided before any run. The list covers three kinds:

| Kind             | Example                                        | What it tests     |
| ---------------- | ---------------------------------------------- | ----------------- |
| Named lookup     | "Where is `parse_config` defined?"             | symbols           |
| Unnamed lookup   | "Where are failed requests retried?"           | notes, and search |
| Concept question | "Why does this function copy its input first?" | notes             |

Named lookups are where plain text search is already strong. A task list made only of them
says little, so the three kinds are reported separately.

### Arms

The same model and the same prompt run every task under each condition.

| Arm | Condition                       | What it answers            |
| --- | ------------------------------- | -------------------------- |
| A   | text search and file reads only | the baseline               |
| B   | Loci with symbols only          | what the index saves       |
| C   | Loci with symbols and notes     | what the notes save on top |

An agent does not behave the same way twice, so each task runs at least three to five times
per arm.

### What is recorded per run

- **Tokens** — input and output separately, with cached input listed on its own. Running the
  agent in headless mode with JSON output reports the usage of each run.
- **Tool calls and wall time.**
- **Correctness** — whether the answer hits the known file and line range.
- **Adoption** — whether the agent asked Loci first, or went to text search anyway.

### Costs that must be counted

- **Fixed overhead.** The MCP tool definitions and the skill text are paid in every session.
  They are counted in arms B and C.
- **Writing the notes.** Notes are written by the LLM, which costs tokens. The break-even
  point is the tokens spent writing notes divided by the tokens saved per query: the number
  of queries after which the notes have paid for themselves.
- **An honest baseline.** Arm A lets the agent use text search the way it normally does. A
  baseline that reads whole files inflates every saving.

### What is reported

The headline number is **tokens per correct answer**, per kind of task and per arm. Fewer
tokens with more wrong answers is not a saving.

Beside it:

- the median and spread of tokens per task;
- the share of correct answers per arm;
- the adoption rate in arms B and C;
- the break-even point of the notes.

## 3. Reading the results

| Result                                          | What it means                                             |
| ----------------------------------------------- | --------------------------------------------------------- |
| B beats A on unnamed lookups, not on named ones | expected; the index earns its place on the hard questions |
| C beats B only after many queries               | the notes pay off on code that is read often              |
| Adoption is low                                 | the skill, not the index, is what needs work              |
| Hook overhead is noticeable at the prompt       | a principle is broken: Loci is in the way                 |
| A is as good as B and C on every kind           | the idea does not pay off on this repository              |
