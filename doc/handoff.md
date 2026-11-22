# Measuring the git handoff

English | [繁體中文](handoff.zh-TW.md)

Loci was measured before it was built, on the sessions of one product repository (perxona, a
Python backend and a TypeScript frontend, worked on with Claude Code). This page records what was
measured, how, and what it does not show.

## 1. Where the tokens go

All main sessions of the repository, by token type and by what each turn did:

| Measure                                   | Value                 |
| ----------------------------------------- | --------------------- |
| Cache reads, share of all tokens          | 97.8%                 |
| Context carried per turn                  | 125k to 270k tokens   |
| Tokens per feature branch / bugfix branch | about 78M / about 24M |
| Largest category of turns                 | exploring, 21% to 32% |
| Editing code                              | under 4%              |

A cache read costs a tenth of an input token, but it is paid on every turn for the whole carried
context. Reading less per turn means carrying less: starting a new task without the previous one.

## 2. Offline replay

`loci replay` runs every past session through the rule, as if the user ran `/clear` at each
suggestion, and models the turns after it:

- the context of a later turn is the session's own floor plus the handoff plus what it grew since;
- the first turn after a handoff writes that new prefix to the cache;
- a session that compacted on its own ends the model.

Cost units weigh input 1, cache write 1.25, cache read 0.1 and output 5, as on the price list.

On the 20 sessions still on disk (3,412 turns), with a 300-token handoff:

| Purpose | Tokens now → with Loci | Cost units now → with Loci |
| ------- | ---------------------- | -------------------------- |
| feature | 509.5M → 280.1M (−45%) | 61.2M → 38.7M (−37%)       |
| bugfix  | 158.5M → 108.5M (−32%) | 19.8M → 15.0M (−24%)       |
| other   | 114.4M → 94.1M (−18%)  | 15.9M → 14.0M (−12%)       |
| total   | 782.3M → 482.7M (−38%) | 96.8M → 67.7M (−30%)       |

The rule judged 57 branch switches: 15 suggestions, 11 revisits, 7 long-lived branches and 24
under the threshold. An earlier rule that suggested at every switch fired more often, mostly at
returns to a branch during a rebase or a review, and saved little more; the new-branch rule
cut those out.

The replay is a model, not a run: it cannot tell whether the cleared session would have needed
more turns to find its way again.

## 3. Live A/B runs

To check the model, two real tasks were run again by an agent, headless, twice each way:

- **Without Loci:** the old session resumed from just before the branch switch, with all its
  context.
- **With Loci:** a new session that got only the Loci handoff.

Both got the same prompt (the issue text and the decisions posted on it), the same start commit,
the same model and a cap of 80 or 120 turns. Pushing, `gh`, the network and package installs were
blocked; every run was checked for remote commands afterwards, and the real repository was
unchanged.

### #8119, a backend fix; about 80k context at the switch

| Run | Tokens: without → with Loci | Cost                 | Time        |
| --- | --------------------------- | -------------------- | ----------- |
| 1   | 4.66M → 4.06M (−13%)        | $2.52 → $2.27 (−10%) | 688s → 593s |
| 2   | 5.68M → 5.10M (−10%)        | $2.83 → $2.61 (−8%)  | 777s → 708s |

Both arms passed their own suites and the regression test of the real fix, and failed the same 5
of the real fix's 110 importer tests: details decided later, not in the issue.

### #8386 and #8388, a frontend and backend fix; about 298k context at the switch

| Run | Tokens: without → with Loci | Cost                  | Largest context |
| --- | --------------------------- | --------------------- | --------------- |
| 1   | 48.29M → 15.94M (−67%)      | $15.29 → $6.55 (−57%) | 496k → 245k     |
| 2   | 27.03M → 14.86M (−45%)      | $10.88 → $6.20 (−43%) | 417k → 226k     |

Averaged: tokens −59%, cost −51%, the same wall time. Run 1 without Loci hit the 120-turn cap,
so its cost is a lower bound.

| Check                                       | Without Loci  | With Loci     |
| ------------------------------------------- | ------------- | ------------- |
| Own backend tests                           | 808, 808 pass | 808, 810 pass |
| Own frontend tests                          | all pass      | all pass      |
| The real pull request's backend tests       | 44/44, 44/44  | 44/44, 44/44  |
| Files also changed by the real pull request | 28/31, 30/31  | 27/29, 27/29  |

Each decision posted on the issues was checked in the diff of all four runs, and each was there.
The real pull request's frontend tests failed in part everywhere, because they name its own
helpers and props. The one difference: the runs with Loci left the translations submodule's
pointer alone until its pull request merges, as the project's own skill says; the runs without
Loci moved it to an unmerged local commit.

## 4. What this does not show

- Two runs per case, on two cases: the direction is clear, the size is not.
- Both arms got the issue text in the prompt. In real use the cleared session reads it from the
  tracker, which costs a few turns; the handoff points at the issue for that reason.
- Whether people act on the suggestion. `loci stats` measures that from real use.
