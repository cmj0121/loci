# Survey 3: review comments

Conclusion: [survey §3](../survey.md#3-review-rounds-repeat-team-conventions-nobody-wrote-down).

## Setup

- The latest 100 merged pull requests of the same repository, merged over 2.3 days, with their
  reviews and review threads (read-only API calls).
- Each thread's first comment, and each "changes requested" body, classified by a small model:
  topic, kind (defect, convention, test, design, nit, question, ack) and the general rule it
  enforces. Rules then merged per topic into canonical rules and counted by distinct PRs.
- Coverage: a larger model compared each rule raised in 3 or more PRs with the repository's
  written agent rules and review skills: explicit, partial or missing.
- Git recall: for the same PRs, each review rule was compared with the PR's follow-up commits
  (every commit after the first in the merged branch): stated in a message, applied only in the
  diff, or absent.
- Cost: under $1 in all.

## Review load

| Measure                                        | Value                                    |
| ---------------------------------------------- | ---------------------------------------- |
| reviews                                        | 2,707: 1,524 commented, 1,014 approved, 151 changes requested, 18 dismissed |
| PRs with changes requested                     | **59%**; 33 PRs twice or more; at most 16 times |
| review threads per PR                          | median 12.5, p90 26                      |
| substantive comments (defect, convention, test, design, nit) | 1,579, almost all opened by agent reviewers; people mostly approved |

## What the comments ask for

| Topic                        | Comments | PRs |
| ---------------------------- | -------- | --- |
| naming                       | 240      | 84  |
| tests                        | 221      | 73  |
| comments and docstrings      | 118      | 59  |
| dead code and duplication    | 112      | 56  |
| architecture boundaries      | 103      | 43  |
| API contracts                | 79       | 34  |
| error handling               | 75       | 27  |
| config, deploy and CI        | 71       | 23  |
| security and authorization   | 70       | 22  |
| performance                  | 66       | 32  |

## Repetition and coverage

| Measure                                        | Value                                    |
| ---------------------------------------------- | ---------------------------------------- |
| canonical rules                                | 791                                      |
| rules raised in 3 or more PRs                  | **91, covering 533 comments (34%)**      |
| … stated explicitly in the written rules       | 8 rules, 47 comments                     |
| … stated in part                               | 33 rules, 211 comments                   |
| … **not written at all**                       | **50 rules, 275 comments**               |

The most repeated kinds: a function name must say what it does and every side effect; one name
per concept across code, docs and UI; one definition per constant, default and condition;
comments describe current behaviour, not history; tests for every branch, boundary and fake.

## What git keeps

| Measure                                        | Value                                    |
| ---------------------------------------------- | ---------------------------------------- |
| PRs merged as a two-parent merge               | 99 of 100                                |
| PRs with follow-up commits                     | 93 (530 follow-up commits)               |
| review comments whose rule a follow-up **message** states | **617 of 1,573 (39%)**        |
| … applied only in a follow-up diff             | 163 (10%)                                |
| … absent from git                              | 793 (50%): folded before merge, declined, or lost in a long diff |

## Takeaways

- The cost a memory could cut is review rounds, and the knowledge to cut it exists.
- Half of it is not in the written rules at all, and git keeps part of the rest.
- Rules can be counted by PR to separate team conventions from one-off remarks; naming rules
  may partly be the taste of one reviewer agent.
