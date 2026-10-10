# AI usage log

The product uses no AI in matching or routing (ADR-006). This log covers development only.

| Date | Phase | Tool / model | AI contribution | Human decision |
|---|---|---|---|---|
| 2026-10-07 | Planning | Claude (claude.ai Project) | Normalized PRD into requirements; flagged conflicts | Reviewed; chose Kalshi + Polymarket |
| 2026-10-07 | Research | Claude with web search | Standards, prior art and settlement divergence research | Directed scope; requested a critique of a standards and patent idea; chose the RFC-draft approach |
| 2026-10-07 | Planning | Claude | Drafted planning pack and wireframes | Reviewed |
| 2026-10-09 | Design | Human-authored | Final build requirements | Locked by the author |
| 2026-10-09 | Planning | Claude | Realigned all artifacts to the final build requirements | Review pending |
| 2026-10-09 | Design | Claude | Proposed scheduled refresh and cache design; drafted amendments A1 to A5 | Approved scheduled approach and in-memory feed |
| 2026-10-09 | Design | Claude | Proposed defaults for watch sides, process split and intervals; rebuilt wireframes | Accepted defaults |
| 2026-10-09 | Research | Claude with web search | Confirmed Polymarket US public market data API and Fed coverage; flagged decision-bucket matching risk | Chose Polymarket US; approved Wave 1 gate |
| 2026-10-09 | Build T-00 | Claude Code, claude-opus-5-5 | Scaffolded module, Makefile, guard, acceptance runner, CLI stubs, CI file | Review pending at Wave 0 boundary |
| 2026-10-09 | Build T-01 | Claude Code, claude-opus-5-5 | Wrote card contracts, loaders, validation, SQLite schema and repositories with tests | Review pending at Wave 0 boundary |
| 2026-10-09 | Build T-02 | Claude Code, claude-opus-5-5 | Wrote testkit (fixture transport, faults, NoNetwork, AsOf, TempStore) and fixtures/README.md | Review pending at Wave 0 boundary |
| 2026-10-10 | Build Wave 1 | Claude Code, claude-opus-5-5 (orchestrator + 2 subagents) | Matcher, router, unlock, GitLab CI; subagents built both fetchers, recorded real fixtures and checked the D-18 gate on live data | Owner approved Q-2, Q-3 and batching; D-18 decision pending |
| 2026-10-10 | Build Wave 2-3 | Claude Code, claude-opus-5-5 (orchestrator + 2 subagents) | Orchestrator built T-20 and T-23 and fixed a SQLite concurrency deadlock; subagents built the feed (T-21) and acceptance and red-team tests (T-22) | Owner chose D-19 and D-20 and asked for batching |
| 2026-10-10 | Ship T-30 | Claude Code, claude-opus-5-5 | Ran the real measurement session, computed metrics, audited 10 not_same pairs against the log, checked citations, wrote README verdict, Dockerfile, demo script and seed tool, dry-ran the demo | Owner approved the verdict wording and demo plan; match audit owner review pending |
| 2026-10-10 | Feed vs wireframes, system test | Claude Code, claude-opus-5-5 (orchestrator + 1 audit subagent) | Restyled feed to the wireframes, watch status sidebar, closest refusal first, wireframe canvas updated to as-built; full system test; docs audited against code and corrected | Owner requested the wireframe check and the test |
