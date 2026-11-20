# Survey 2: lessons from past fixes

Conclusion: [survey §2](../survey.md#2-lessons-from-past-fixes-are-sound-but-rarely-prevent-the-next-mistake).

## Setup

- History: the base branch of the same repository from January 2026 on: 17,789 non-merge
  commits by 77 authors, about a third of them co-authored by a coding agent.
- Fix pairs, SZZ-style: for each `fix`/`revert` commit, `git blame` the lines it changed in its
  parent and record the commits that introduced them. Lock files, translations and generated
  code are skipped. Free: git only.
- A post-merge fix = the introducing commit landed on the base branch through an earlier merge
  than the fix. Everything else is an iteration inside one pull request.
- Extraction: one small-model call per fix PR (fix message and diff plus the introducing
  diffs), at most two lessons, evidence quoted verbatim or dropped.
- Judges: a small model, shown known lessons mixed with two decoy lessons from other files, in
  shuffled order. Decoys measure judge bias.

## Fix pairs

| Measure                                        | Value                                              |
| ---------------------------------------------- | -------------------------------------------------- |
| fix commits that change code                   | 5,008                                              |
| fixes inside one PR                            | most; median lag about 1 hour, 93% by the same author |
| **fixes of code merged by an earlier PR**      | **1,917 commits in 1,171 PRs**; median lag 11.5 days (p25 3.8, p75 27.2); 40% by another author |

## Lessons

| Measure                                        | Value                                              |
| ---------------------------------------------- | -------------------------------------------------- |
| lessons from 1,161 fix PRs                     | 1,520, **$2.79** (~$0.0024 per PR)                 |
| dropped for non-verbatim evidence              | 242                                                |
| state of their anchors at HEAD                 | 960 valid, 275 stale, 269 gone                     |
| useful, from a hand-read sample of 25          | ~20 (80%); 4 low value; 1 confused                 |

## Would they have helped?

| Test                                                                 | Result                                       |
| -------------------------------------------------------------------- | -------------------------------------------- |
| prompt hook, first prompt of 22 sessions                             | injected in 21; on a file the session touched in 1 |
| back-test, 400 later post-merge bugs: a lesson existed for the file when the bug was written | 70 (17.5%) |
| … prompt hook on the introducing commit's subject, same file         | 9 (2.3%)                                     |
| judge, the 70 cases: known lessons (190)                             | prevent 3 · related 106 · no 81              |
| judge, decoys (140)                                                  | prevent 0 · related 12 · no 128              |
| → later bugs a known lesson would have prevented                     | **3 of 400 (0.75%)**, $0.09                  |
| in-PR fix rounds (8,127 events, 1,321 PRs; 400 sampled, 85 with a prior lesson): known lessons (203) | prevent 3 · related 105 · no 95 |
| … decoys (170)                                                       | prevent 1 · related 15 · no 154              |
| → in-PR fix rounds a known lesson would have avoided                 | **about 2 of 400 (0.5%)**, $0.10             |
| file-triggered injection: 2 lessons per first touch of a file, 22 sessions | 2,070 touches, 387 injections (~39k tokens), 17 on files later edited |
| … judge, 100 sampled injections                                      | useful 2 · related 24 · noise 74, $0.04       |

## History lookup cost

| Measure                                                   | Value                                        |
| --------------------------------------------------------- | -------------------------------------------- |
| model input over the 22 sessions                          | 2.17B tokens                                 |
| history lookups (`git log/show/blame`, PR and issue reads, review threads) | 985 calls, ~74.7M tokens carried in context, **3.45%** |
| digging into why code is as it is                         | `git blame` once; `git log -- <path>` 17 times |
| lookups whose target already had a lesson                 | 14 calls, 0.07% of model input               |

Carried tokens count a result once, plus once for every later model call of the same agent, since
it stays in context.

## Takeaways

- Fix history is a rich, cheap source of correct team knowledge.
- Delivered automatically, it rarely fits the work at hand: lessons are "related" half the time
  but almost never the one the next change needed.
- Agents do not pay to rediscover history; they skip it. A lesson adds knowledge, not savings.
