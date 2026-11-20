# Loci survey

English | [繁體中文](survey.zh-TW.md)

After the gates closed Loci as a code-lookup tool ([Measurement](measurement.md)), the same
question was asked of every other kind of memory an agent could be given: does it save tokens on
a real team repository? Each section states one conclusion; the numbers behind it are in
`doc/results/`. All measurements ran on one large repository worked on by many people and
coding agents; it is not named here.

## 1. Agents rarely re-explore, so session memory has little to save

Exploration is most of what agents read, but almost all of it is first-time reading for the task
at hand. Re-reading an unchanged file in a later session is about 3% of exploration, and a
memory captured from past sessions is mostly noise about the tool environment. A memory of past
sessions has little to cut. [Results](results/survey-1-exploration.md)

## 2. Lessons from past fixes are sound, but rarely prevent the next mistake

A fix of code that an earlier pull request merged is a real lesson, and a small model turns most
of them into accurate, reusable rules. But the next mistake is almost always a new one: a lesson
known before a later bug was written would have prevented about 0.75% of those bugs and about
0.5% of in-PR fix rounds. Agents also hardly dig into history (3.45% of model input), so there
is no lookup cost to replace. [Results](results/survey-2-fix-lessons.md)

## 3. Review rounds repeat team conventions nobody wrote down

The visible cost in a busy team repository is the review round trip, not finding code: most
pull requests are sent back at least once. A third of review comments repeat a rule already
raised in three or more pull requests, and most of those rules are not in the repository's own
agent rules. Git keeps part of this knowledge: a follow-up commit message states the rule for
39% of review comments. [Results](results/survey-3-reviews.md)

## 4. A pre-push rule check finds more, but saves no review rounds

Rules mined from git and selected by the paths a change touches roughly double what a plain
pre-push check catches, and dumping every rule as text does worse than no rules at all. But a
review round holds several requests, so catching one rarely removes the round: the check avoided
2.7% of rounds against 1.8% with no rules, and it cost more tokens than it saved. Most mined rules
also never recur: 96% appear in one pull request. [Results](results/survey-4-precheck.md)

## 5. What Loci keeps: deterministic convention checks and the eval method

No memory delivered to an agent paid for itself, and learning review conventions from history is
already a product category in 2026. What remains worth keeping is narrow: mining the few
conventions that recur, and turning each into a written rule or a deterministic check that runs
on every change; and the eval method itself (time cutoffs, no-rules and plain-text controls, a
blind judge, review rounds, measured round cost), which existing tools cover only in part. Loci is
not continued as a product. [Results](results/survey-5-tools.md)
