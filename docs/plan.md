# Plan

Waves run in order. A wave starts only when the previous wave is merged and `make test lint guard` passes. Tasks within a wave never share files.

| Wave | Tasks | Harness | Why |
|---|---|---|---|
| 0 contracts | T-00, T-01, T-02 (serial) | Claude Code | Types, schema, vocab and fixtures every worker codes against |
| 1 core | T-10, T-11, T-12, T-13 (parallel) | AO workers | Four independent pieces against fixed contracts |
| gate | D-18 Fed-pair check | - | At least one real Fed pair on Kalshi and Polymarket US fits the cutoff format, or the owner decides the fallback |
| 2 wiring | T-20, T-21, T-22 (parallel) | AO workers | Commands, feed, end-to-end evidence |
| 3 background | T-23 | Claude Code | Scheduler reuses fetch, match and route; single task, no parallelism needed |
| 4 ship | T-30 | Claude Code | Results, README verdict, demo |

Critical path: T-00 -> T-01 -> T-02 -> (T-10, T-11) -> T-22 (test 16) -> T-23 -> T-30.
Matcher (T-12) and router (T-13) are built entirely on hand-built cards and prices and do not wait for fetchers.
