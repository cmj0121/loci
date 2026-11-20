# Survey 5: existing tools (November 2026)

Conclusion: [survey §5](../survey.md#5-what-loci-keeps-deterministic-convention-checks-and-the-eval-method).

Collected from vendor documentation, changelogs and papers on 2026-11-20. Claims come from
those sources and were not tested here; "?" means not found.

## Learning review conventions

Columns: (a) mines past PR comments in bulk, (b) learns from new feedback, (c) rules a person
curates, (d) produces deterministic checks, (e) works from git alone, (f) measures whether
comments repeat less.

| Tool            | a                          | b                   | c                                   | d   | e  | f                                  |
| --------------- | -------------------------- | ------------------- | ----------------------------------- | --- | -- | ---------------------------------- |
| Qodo            | yes, ~1,000 PRs, every 2 weeks [1] | yes [2]     | pending rules in UI; editable wiki file [1][2] | ? | no | rule analytics only [1]  |
| Cursor Bugbot   | backfill of recent PRs [3] | yes [3]             | dashboard, not versioned [4]        | ?   | no | resolution rate, judged by an LLM [3] |
| Cubic           | yes, chosen reviewers [5]  | yes [5]             | ?                                   | ?   | no | ?                                  |
| Kodus           | yes, last 3 months [6]     | yes [6]             | approval queue [6]                  | ?   | ?  | ?                                  |
| Ellipsis        | yes [7]                    | yes [7]             | UI                                  | ?   | no | ?                                  |
| CodeRabbit      | import only [8]            | yes [8]             | dashboard; reads agent rule files [8][9] | ? | no | usage counts [8]               |
| Greptile        | ? [10]                     | yes [10]            | inferred rules [11]                 | ?   | no | addressed vs ignored comments [10] |
| GitHub Copilot  | no                         | memory from agent work, 90-day expiry [12] | hand-written files [13] | ? | no | ?                   |
| Bito, Augment, Gitar | no / ? [14][15][16]   | yes                 | file or curated memory              | ?   | no | ?                                  |
| Graphite, Sourcery, Claude Code review | no [17][18][19] | no         | hand-written rules                  | hand-written static rules (Sourcery) | no | issues per rule (Graphite) |
| Semgrep Assistant | n/a                      | triage memories, security only [20] | admin-approved          | does not generate rules [20] | no | ? |

Not covered by any of them: mining from git alone; rules kept as versioned files; turning a
convention into a lint or CI check (only community skills do this [21][22]); back-testing a rule
on held-out PRs; measuring whether the same *human* comment recurs; handing rules to the author's
own agent.

## Measuring whether agent context helps

Columns: (a) replays the team's own PRs, (b) time-cutoff leakage guard, (c) control arms,
(d) blind judge with a human audit, (e) tokens, cost and time, (f) review rounds, (g) statistics.

| Tool                     | a       | b                          | c                              | d                         | e                 | f  | g                      |
| ------------------------ | ------- | -------------------------- | ------------------------------ | ------------------------- | ----------------- | -- | ---------------------- |
| `claude plugin eval` [23] | partial | no                         | with vs without the plugin     | LLM votes; no audit found | cost, duration    | no | mean of 3 runs         |
| mdarena [24]             | yes     | history-free checkout      | bare + several context files   | tests, no judge           | tokens, cost      | no | paired t-test          |
| agent-pr-replay [25]     | yes     | ?                          | no                             | LLM diff comparison       | cost estimate     | no | usage only             |
| promptfoo [26]           | no      | no                         | any arms                       | rubric                    | tokens, cost, latency | no | repeats only       |
| Inspect AI [27]          | build your own | no                  | build your own                 | model graders             | token logs        | no | standard error         |
| Braintrust, LangSmith, Phoenix, DeepEval [28] | no | no      | experiments                    | yes                       | from traces       | no | ?                      |
| this repository (`loci eval`, `loci eval precheck`) | yes | per-cutoff rebuilt store | none and plain-text controls | blind, audit export | tokens, cost, time; weighted | yes | paired bootstrap CI |

## Published findings on context for coding agents

- LLM-written context files: −0.5 to −2 points on two benchmarks, not significant;
  developer-written: +2.4 points (p = 0.21); cost +20–23% [29].
- AGENTS.md on 124 PRs: median runtime −29%, output tokens −17%, completion "comparable", no
  significance tests [30].
- Skills: curated skills +16 points, self-written about 0 [31]; 39 of 49 skills gave no gain,
  mean +1.2 points [32]; one vendor reports an 8 KB AGENTS.md index beating skills [33].

These agree with the survey: most added context does not move the result, and it costs tokens.

## Takeaways

- Learning conventions from reviews is crowded; what is left is narrow (git-only, versioned
  rules, deterministic checks, back-testing), so it is not a product.
- The eval method is more complete than existing tools on leakage, controls, judging and review
  rounds; it is best offered as a method or a contribution to an existing replay tool.

## Sources

[1] https://docs.qodo.ai/governance/rule-enforcement/rule-miner ·
[2] https://qodo-merge-docs.qodo.ai/core-abilities/auto_best_practices ·
[3] https://cursor.com/blog/bugbot-learning ·
[4] https://forum.cursor.com/t/api-or-git-based-workflow-to-promote-bugbot-learned-rules-into-repository-rules/171018 ·
[5] https://docs.cubic.dev/ai-review/memory-and-learning ·
[6] https://docs.kodus.io/how_to_use/en/code_review/learning/kody_rules_generation.md ·
[7] https://docs.ellipsis.dev/reviews ·
[8] https://docs.coderabbit.ai/knowledge-base/learnings ·
[9] https://docs.coderabbit.ai/knowledge-base/code-guidelines ·
[10] https://www.greptile.com/docs/how-greptile-works/memory-and-learning ·
[11] https://www.greptile.com/docs/code-review-bot/best-practices ·
[12] https://docs.github.com/en/copilot/concepts/agents/copilot-memory ·
[13] https://github.blog/changelog/2026-07-17-copilot-code-review-customization-and-configurability-improvements/ ·
[14] https://docs.bito.ai/ai-code-review-agent/implementing-custom-code-review-rules ·
[15] https://docs.augmentcode.com/cosmos/experts-code-review-memory ·
[16] https://www.sonarsource.com/products/gitar/features/ ·
[17] https://graphite.com/docs/ai-review-customization ·
[18] https://docs.sourcery.ai/reviews/review-rules.md ·
[19] https://code.claude.com/docs/en/code-review ·
[20] https://semgrep.dev/docs/semgrep-assistant/customize ·
[21] https://tessl.io/registry/tessl/review-miner ·
[22] https://skillsmp.com/creators/zernie/vigiles/claude-skills-pr-to-lint-rule ·
[23] https://code.claude.com/docs/en/plugin-evals ·
[24] https://github.com/HudsonGri/mdarena ·
[25] https://github.com/sshh12/agent-pr-replay ·
[26] https://promptfoo.dev/docs/guides/evaluate-coding-agents/ ·
[27] https://inspect.aisi.org.uk/ ·
[28] https://www.braintrust.dev/docs/observe/dashboards/coding-agent-insights.md ·
[29] https://arxiv.org/abs/2602.11988 ·
[30] https://arxiv.org/abs/2601.20404 ·
[31] https://arxiv.org/abs/2602.12670 ·
[32] https://arxiv.org/abs/2603.15401 ·
[33] https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals
