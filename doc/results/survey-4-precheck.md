# Survey 4: a pre-push rule check (POC 2)

Conclusion: [survey §4](../survey.md#4-a-pre-push-rule-check-finds-more-but-saves-no-review-rounds).

POC 2 is on branch `feat/loci-review` (226 tests, no real LLM in tests): `loci mine` learns
rules from git, `loci precheck` checks a branch's diff against the rules for its paths, and
`loci eval precheck` measures it.

## How rules are mined

- A pull request in git = a first-parent merge on the base branch; its commits are
  `M^1..M^2`; the first is the initial commit, the rest are follow-ups.
- Sources: `Review-Rule:` trailers (none existed yet), follow-up commits, and fixes of code an
  earlier merge brought in (blame). One small-model call per PR, at most three rules, evidence
  verbatim from a message or a `+`/`-` line (≥ 12 chars), paths taken from the changed files.
- Canonical rules join only when the text is the same requirement; a rule's count is its
  distinct merges.

| Measure                              | Value                                             |
| ------------------------------------ | ------------------------------------------------- |
| PRs mined (about five weeks)         | 972, 5 failed (malformed reply or git error), retried later |
| rules                                | 1,821 from 1,326 sources; 193 dropped for evidence |
| PRs per rule                         | 1: 1,754 · 2: 56 · 3: 9 · 6: 1 · 12: 1            |
| cost                                 | $2.48, plus $0.50 for a sizing slice (~$0.0037 per PR) |

Because so few rules recur, the eval used every rule (`--min-prs 1`); this was fixed before any
eval result was seen.

## How the check was measured

Rules fixed before running, from two rounds of plan challenge:

- **Leakage:** three cutoffs (T = 2026-11-01, 11-08, 11-14). For each, a separate store rebuilt
  from sources whose commit and merge are both before T, by exact-text joins only, never from
  the PR under test.
- **Sample:** 30 random merges per cutoff from non-overlapping windows after T, including PRs
  with no follow-ups (90 PRs, 80 with follow-ups).
- **Rounds:** follow-ups split into review rounds by a 30-minute gap (15 and 60 also reported);
  each follow-up marked review-driven or other in a separate call, before any finding exists.
- **Arms**, each on the PR's initial diff: `none` (the same prompt, no rules), `plain` (every
  rule as plain text, same size budget), `loci` (rules covering the changed paths).
- **Blind judge:** one call per PR with the diff, the follow-ups and all arms' findings
  shuffled and unlabelled. A round is avoided only when every review follow-up in it was
  caught.
- **Cost of a round:** measured from 47 review-fix segments of agent transcripts, priced on one
  scale (input 1×, cache write 1.25×, cache read 0.1×, output 5×).
- **Pass rule:** `loci` − best control ≥ 5 points of rounds avoided with a bootstrap CI above 0;
  tokens saved > 0 at the median round cost after charging the check on every push; judge–hand
  agreement ≥ 80% per group.

## Results

| Arm     | Review rounds | Avoided | Rate     | Recall   | Precision (judge) | Findings per PR | Cost per push       |
| ------- | ------------- | ------- | -------- | -------- | ----------------- | --------------- | ------------------- |
| `none`  | 112           | 2       | 1.8%     | 2.1%     | 41.7%             | 0.14            | 12.6k tokens, $0.0021 |
| `plain` | 112           | 1       | 0.9%     | 0.8%     | 16.7%             | 0.09            | 14.0k tokens, $0.0024 |
| `loci`  | 112           | 3       | **2.7%** | **4.7%** | **48.4%**         | 0.38            | 11.0k tokens, $0.0020 |

| Measure                                     | Value                                             |
| ------------------------------------------- | ------------------------------------------------- |
| `loci` − `none`, rounds avoided (30 min)    | **+0.9 points** (95% CI +0.0 to +3.0); +0.0 at 15 min, +1.0 at 60 min |
| cost of a review round (weighted)           | median 285k tokens (p25 135k, p75 789k)           |
| break-even                                  | the check pays if it avoids ~4% of rounds         |
| tokens saved by `loci` at the median round  | **−1.98M** over the sample (a net cost)           |
| smallest difference this sample can detect  | ~14 points pooled                                 |
| eval cost                                   | $0.83                                             |

**Pass rule:** rounds FAIL, tokens FAIL; the audit was not completed because it could not change
the verdict. **Verdict: FAIL.**

A hand read of the 15 `loci` findings the judge marked caught found all of them real: 8 were
one convention (an API schema changed without regenerating the published spec and client),
2 a docstring-length rule, and the rest genuine bugs a reviewer later fixed.

## Takeaways

- Selecting rules by path helps; more rules as plain text hurts.
- A check that finds one item per round does not remove rounds, so it does not save tokens.
- The rules that pay are the few that recur, and those are better as written rules or as
  deterministic checks (a schema change without a regenerated spec can be caught by a diff
  rule) than as an LLM check on every push.
