# Measuring Loci

English | [繁體中文](measurement.zh-TW.md)

Loci makes two promises: it stays out of the way, and it lets an agent reach the code with
fewer tokens. This document says how each is measured and what the first measurement found.
The behaviour under test is defined in the [specification](spec.md).

Both measurements can run before Loci exists, with an existing index tool standing in for it.
That is the cheapest way to learn whether the idea pays off.

## 1. Speed

**Purpose:** show that Loci never makes the developer wait.

| Metric        | What it is                                    | How it is measured                             |
| ------------- | --------------------------------------------- | ---------------------------------------------- |
| Hook overhead | time a hook adds to `git commit`              | same commit with and without the hook; the gap |
| Build time    | from the hook firing to the memory published  | two timestamps recorded by Loci                |
| Stale window  | how long a query still gets the old memory    | that commit's build time, worst case           |
| Query latency | from an MCP call being sent to its answer     | hundreds of calls per tool; p50 and p95        |
| Switch time   | from `checkout` to answering the new revision | switching between two built branches           |
| Storage       | disk used by a repository's memory            | size per 1,000 files; growth per 100 commits   |

Build time is reported for a first full build and for a single commit.

**How:** replay the real last N commits of a small, a medium and a large repository. Report
the median of many runs with the worst case beside it, on one named machine.

**Rule:** a hook overhead the developer notices at the prompt breaks the principle; Loci is in
the way.

## 2. Token usage

**Purpose:** show whether an agent with Loci spends fewer tokens than the same agent without
it, on the same tasks.

**Tasks.** A fixed list on a real repository, each with a known answer (a file and a line
range) set before any run. The three kinds are reported separately:

| Kind             | Example                                        | What it tests     |
| ---------------- | ---------------------------------------------- | ----------------- |
| Named lookup     | "Where is `parse_config` defined?"             | symbols           |
| Unnamed lookup   | "Where are failed requests retried?"           | notes, and search |
| Concept question | "Why does this function copy its input first?" | notes             |

Unnamed lookups and concept questions must not carry a word only the answer's code contains:
check each with `rg` and reword on a unique hit. Run the list on more than one repository size.

**Arms.** Each arm is an interface to the same memory, with the same model and prompt.

| Arm | Interface          | What the agent has                                     | What it answers                 |
| --- | ------------------ | ------------------------------------------------------ | ------------------------------- |
| A   | text search only   | text search and file reads                             | the baseline                    |
| B   | MCP                | Loci's symbol tools and the skill                      | what the index saves            |
| C   | MCP                | all Loci tools, the skill and the notes                | what the notes save on top      |
| D   | skill + CLI        | the `loci` CLI, its skill and the notes; no MCP        | whether a CLI avoids MCP's cost |
| E   | skill + CLI + hook | D plus a prompt hook that injects suggestions          | whether pushed suggestions help |
| E0  | skill + CLI + hook | E without notes: the CLI and the hook match names only | what the notes add to E         |

**Runs.** Each task runs 5 times per arm, interleaved (A, E, E0, A, E, E0, ...), so drift hits
every arm alike. Each run gets a fresh `git worktree`, an empty agent config and a fresh copy
of the notes. The answers, user MCP servers, plugins, edits, web access and subagents are out
of reach.

**What is counted.** Per run: input, output and cached tokens, cost, tool calls, wall time,
correctness, and adoption (whether the first locating call was Loci). Every arm pays its own
fixed overhead (tool definitions, skill text) and, for MCP arms, the ToolSearch turn that loads
the tools. Writing the notes counts too: their break-even point is the tokens spent writing
them divided by the tokens saved per query. Arm A uses text search as the agent normally does.

**Rule.** The headline is **tokens per correct answer**, per kind and arm, with **cost per
run** beside it, since cheap cached reads can hide a token saving. Each arm − A difference
gets a bootstrap 95% CI over runs within a task; a CI that includes 0 is not a result. The
rule is written down before the run.

## 3. Gate 0 results

- Gate 0's rule (beat text search on unnamed lookups) was not met in any of five rounds. Loci
  does not pay off as a faster way to find code: text search finds a function in 1–2 calls,
  on 53k lines and on 596k.
- What pays off is notes on concept questions, handed over by a prompt hook. In r5 on ripgrep,
  E saved 16,157 tokens per correct answer and $0.0020 per run, both CIs excluding 0, and was
  correct in 77% of runs against 70%. Without notes (E0) there is no clear gain.
- MCP does not fit: in Claude Code its tools sit behind ToolSearch, and the extra turn costs
  more than the tools save.
- The open cost is the note pass: $13.77 for ripgrep, repaid after about 6,900 concept
  questions, so notes must be written on demand. Unproven: notes on a large repository, and
  real Loci, since every round used a stand-in.
- So the [MVP becomes](#mvp-changes) a prompt hook and CLI context provider with notes on
  demand, and no MCP.

### Setup

A Python stand-in for Loci ran five rounds, with Sonnet 5.5 in every arm. The full reports are
the evidence:

- r1, arms A, B and C: [`results/gate0-r1.md`](results/gate0-r1.md);
- r2 (B and C again) and r3 (D and E), A reused from r1:
  [`results/gate0-r2-r3.md`](results/gate0-r2-r3.md);
- r4, interleaved, 5 runs per task, with CIs: [`results/gate0-r4.md`](results/gate0-r4.md);
- r5, concept questions only, reasons scored too: [`results/gate0-r5.md`](results/gate0-r5.md).

| Repository    | Version    | Rust files | Rust lines | Tasks           | Rounds |
| ------------- | ---------- | ---------- | ---------- | --------------- | ------ |
| ripgrep       | 15.2.0     | 100        | 53k        | 12, +12 concept | r1–r5  |
| rust-analyzer | 2026-11-16 | 1,510      | 596k       | 12, +12 concept | r4–r5  |

Every answer in r1–r4 was correct, so there tokens per correct answer compares effort alone.

### r1–r3: ripgrep, 3 runs per task

Tokens per correct answer; no CI and no cost.

| Round | Arm | Interface          | Unnamed | All kinds |
| ----- | --- | ------------------ | ------- | --------- |
| r1    | A   | text search only   | 52,808  | 50,577    |
| r1    | B   | MCP                | 58,669  | 68,104    |
| r1    | C   | MCP                | 58,748  | 59,564    |
| r2    | B   | MCP                | 66,775  | 69,207    |
| r2    | C   | MCP                | 63,459  | 63,134    |
| r3    | D   | skill + CLI        | 60,781  | 53,507    |
| r3    | E   | skill + CLI + hook | 52,362  | 43,594    |

- Both MCP arms spent more than A on every kind. Each first MCP use cost a ToolSearch turn,
  and each turn re-reads about 45k tokens of context.
- E led all kinds. It passed the unnamed gate by 446 tokens, without a CI.

### r4: interleaved, 5 runs per task

Tokens per correct answer for A, and arm − A with its 95% CI. Negative means cheaper than A;
bold marks a CI that excludes 0.

ripgrep:

| Kind    | A      | E − A                         | E0 − A                      |
| ------- | ------ | ----------------------------- | --------------------------- |
| named   | 37,794 | **-5,899 [-7,674, -4,122]**   | **-7,863 [-7,875, -7,851]** |
| unnamed | 52,920 | **+3,017 [+1,022, +5,836]**   | +2,022 [-2,685, +7,155]     |
| concept | 58,230 | **-12,880 [-15,989, -9,522]** | -235 [-6,472, +6,580]       |
| all     | 49,648 | **-5,254 [-6,730, -3,731]**   | -2,025 [-4,600, +749]       |

rust-analyzer, arms A and E0 only, so no note pass:

| Kind    | A      | E0 − A                        |
| ------- | ------ | ----------------------------- |
| named   | 40,434 | **-5,949 [-8,834, -3,050]**   |
| unnamed | 57,452 | **+5,696 [+1,920, +8,945]**   |
| concept | 76,422 | **-12,010 [-22,136, -2,801]** |
| all     | 58,103 | **-4,088 [-7,651, -451]**     |

Mean cost per run, all kinds:

| Repository    | Arm | Cost / run $ | Arm − A $                  |
| ------------- | --- | ------------ | -------------------------- |
| ripgrep       | A   | 0.0381       | —                          |
| ripgrep       | E   | 0.0368       | -0.0013 [-0.0032, +0.0003] |
| ripgrep       | E0  | 0.0374       | -0.0007 [-0.0026, +0.0009] |
| rust-analyzer | A   | 0.0453       | —                          |
| rust-analyzer | E0  | 0.0448       | -0.0005 [-0.0018, +0.0007] |

- Unnamed lookups got worse: E and E0 spent more than A on both repositories.
- Concept questions saved about 12k tokens: with notes on ripgrep (E), without on
  rust-analyzer (E0). E0 on ripgrep saved nothing clear. E did not run on rust-analyzer.
- Costs fell only 1–3% per run, CIs including 0. The one clear cost saving was E on ripgrep
  concept questions (-$0.0059 per run).
- The D14 rule was met formally: E0 beat A on rust-analyzer for all kinds. But the upper bound
  is near 0, E0 lost on unnamed lookups, and its cost CI includes 0.
- Text search scales well: A's all-kinds tokens grew 17% (49,648 to 58,103) for 11 times the
  code. The task lists differ, so this is rough.

### r5: concept questions, 5 runs per task

12 new concept questions per repository. An answer is correct when its `ANSWER: path:line`
falls in the task's line range and the reply names every listed reason (`must_mention`, with
aliases). Rule D15: an arm beats A when the CIs of its tokens per correct answer and its cost
per run both lie below 0, and its correct share is no lower. rust-analyzer has no notes, so it
ran A and E0 only.

| Repository    | Arm | Correct  | Tokens / correct | Arm − A                       |
| ------------- | --- | -------- | ---------------- | ----------------------------- |
| ripgrep       | A   | 42 (70%) | 91,689           | —                             |
| ripgrep       | E   | 46 (77%) | 75,532           | **-16,157 [-24,905, -8,741]** |
| ripgrep       | E0  | 42 (70%) | 83,804           | -7,885 [-18,394, +2,801]      |
| rust-analyzer | A   | 46 (77%) | 90,814           | —                             |
| rust-analyzer | E0  | 49 (82%) | 84,159           | -6,655 [-21,088, +5,702]      |

| Repository    | Arm | Cost / run $ | Arm − A $                      | D15      |
| ------------- | --- | ------------ | ------------------------------ | -------- |
| ripgrep       | A   | 0.0496       | —                              | —        |
| ripgrep       | E   | 0.0475       | **-0.0020 [-0.0034, -0.0007]** | beats A  |
| ripgrep       | E0  | 0.0481       | -0.0014 [-0.0031, +0.0003]     | does not |
| rust-analyzer | A   | 0.0563       | —                              | —        |
| rust-analyzer | E0  | 0.0561       | -0.0002 [-0.0027, +0.0021]     | does not |

Mean turns on ripgrep: A 4.0, E 3.5, E0 3.7. The round cost $15.46.

### Task defects

Four tasks failed in nearly every run of every arm, three on ripgrep and one on rust-analyzer.
The agent found the right file but pointed at a neighbouring function, or missed a required
wording. The defect is in the task, not the arm. Scored at file level, the order holds: tokens
per correct answer on ripgrep were A 75,509, E 62,044, E0 70,395; on rust-analyzer A 77,360,
E0 71,100. Cost per run does not depend on scoring.

### The note pass

Notes for ripgrep took 2 sessions, about 53M tokens (52,969,105) and $13.77. They cover 2,090
of 2,165 non-test functions. Against B they break even after 6,203 queries (r1) or 8,721
(r2). Against A, at E's r5 saving of $0.0020 per concept question, after about 6,900.

### Adoption

The agent called `loci` first in 14% (E) and 22% (D) of r3 runs. In r4 it was 22% (E) and 35%
(E0) on ripgrep, and 20% (E0) on rust-analyzer. The r4 figures come from the raw run records;
`results/gate0-r4.md` does not print adoption.

### MVP changes

| Was                                | Becomes                                                    |
| ---------------------------------- | ---------------------------------------------------------- |
| six MCP tools                      | a prompt hook and a CLI; no MCP                            |
| a symbol search that finds code    | a context provider that explains code                      |
| notes for every function, up front | notes on demand: functions asked about or recently changed |
| one behaviour on every repository  | passive (build only) on small ones, active on large ones   |
