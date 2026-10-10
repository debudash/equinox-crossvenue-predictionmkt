# Task board

Status: todo | in-progress | in-review | blocked | merged. Updated after every state change. Detail in docs/tasks/T-xx.md.

| ID | Title | Wave | Required tests | Harness / agent | Branch | PR | Status | Blocker |
|---|---|---|---|---|---|---|---|---|
| T-00 | Scaffold, CI, guard, acceptance runner | 0 | runner for 1-20 | Claude Code (Opus 5.5) | t-00-scaffold | !1 | in-review | owner merge |
| T-01 | Card types, vocab, config, SQLite store | 0 | supports 17 | Claude Code (Opus 5.5) | t-01-contracts | !2 | in-review | owner merge |
| T-02 | Fixtures and test kit | 0 | supports all | Claude Code (Opus 5.5) | t-02-testkit | !3 | in-review | owner merge |
| T-10 | Kalshi fetcher | 1 | 17, 20 (fetch side) | Claude Code subagent (Opus 5.5) | wave-1 | wave-1 MR | in-review | D-18 gate failed; owner decision |
| T-11 | Polymarket US fetcher | 1 | 17, 20 (fetch side) | Claude Code subagent (Opus 5.5) | wave-1 | wave-1 MR | in-review | D-18 gate failed; owner decision |
| T-12 | Matcher | 1 | 1-7, 17 (match side) | Claude Code (Opus 5.5) | wave-1 | wave-1 MR | in-review | D-18 gate failed; owner decision |
| T-13 | Router | 1 | 8-15 | Claude Code (Opus 5.5) | wave-1 | wave-1 MR | in-review | D-18 gate failed; owner decision |
| T-20 | Fetch and rebuild commands, run lock | 2 | 18, W7 | Claude Code (Opus 5.5) | wave-1 | !4 | in-review | |
| T-21 | Read-only feed and cache | 2 | W5, W6, W10 | Claude Code subagent (Opus 5.5) | wave-1 | !4 | in-review | |
| T-22 | Integration and red-team tests | 2 | 16, 17, 19, 20 (end to end) | Claude Code subagent (Opus 5.5) | t-22-acceptance | | in-progress | |
| T-23 | Background watch scheduler | 3 | W1-W4, W8, W9, W11 | Claude Code (Opus 5.5) | wave-1 | !4 | in-review | |
| T-30 | Evidence and ship | 4 | 20 of 20, W1-W11 | Claude Code | | | todo | |
