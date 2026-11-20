# Survey 1: exploration cost and session memory

Conclusion: [survey §1](../survey.md#1-agents-rarely-re-explore-so-session-memory-has-little-to-save).

## Setup

- Repository: one large team repository, many people and coding agents, growing fast.
- Data: 22 Claude Code sessions of one developer over 9 days, main agents and subagents, read
  from their transcripts. Free: no LLM calls.
- Exploration = `Read`, `Grep`, `Glob`, and shell `grep`, `sed`, `cat`, `ls`, `find`, `head`,
  `tail`; history = `git log`, `git show`, `git blame`, `git diff`.
- A re-read = the same file read again in a later session; it is "changed" when a commit touched
  the file between the two reads.

## Where tool results go

| Measure                                        | Value                                         |
| ---------------------------------------------- | --------------------------------------------- |
| tool calls                                     | 12,162 (7,483 in subagents)                   |
| exploration + history calls                    | 44% (5,327)                                   |
| their share of all tool-result text            | 72% (~20.9M chars, ~5.2M tokens)              |
| most used                                      | shell `grep` 1,605, `sed` 1,509, `cat` 664; `Read` 132 |
| exploration before the first edit of a task    | median 4 calls (p75 12), median ~2.6k tokens  |

## Re-exploration across sessions

| Measure                                        | Value                               |
| ---------------------------------------------- | ----------------------------------- |
| files read                                     | 457                                 |
| files read in 2 or more sessions               | 148                                 |
| cross-session re-reads                         | 307                                 |
| … of a file changed in between                 | 181 (59%), ~5% of exploration text  |
| … of an unchanged file                         | 126 (41%), **~3% of exploration text** |
| search patterns repeated across sessions       | 57 of 1,395 (4%)                    |

The hottest areas were re-read in 14–15 of the 22 sessions, mostly because they kept changing.

## Session memory (POC 1)

POC 1 captured findings and wrong-try lessons from each session's transcript with a small model,
anchored them to function body hashes, and injected matching ones on each prompt.

| Measure                                        | Value                                                  |
| ---------------------------------------------- | ------------------------------------------------------ |
| memories from the 22 transcripts               | 64: 33 findings, 31 lessons                            |
| lessons accurate by hand                       | 29 of 31 (94%)                                         |
| near-duplicates of one shell-environment lesson | 11 (unanchored ones were deduplicated per session only) |
| lessons about the agent's tool environment     | 16, unanchored, so never injected                      |
| lessons about the repository itself            | ~14                                                    |
| replay eval                                    | not run: the 30 history prompts after the cutoff were mid-conversation follow-ups ("push it", "check again") that mean nothing replayed alone; it would have cost ~$75 |

## Takeaways

- Exploration is expensive, but it is first-time reading; what repeats is mostly a file that
  changed in between, which a stored memory would have had wrong.
- The ceiling for a session memory is ~3% of exploration text before any retrieval loss.
- Prompts in team work are often short follow-ups or issue links, so prompt-word retrieval has
  little to match.
