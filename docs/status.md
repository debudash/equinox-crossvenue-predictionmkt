# Status

Current wave: 2 (Wave 1 on branch wave-1, MR !4; D-18 resolved by D-19 and D-20)

## Log
- 2026-10-07: Planning pack generated.
- 2026-10-09: Design locked in docs/build-requirements.md; all artifacts realigned.
- 2026-10-09: Amendments A1 to A5 approved: watch scheduler, cycles, in-memory feed cache, new-data banner, retention.
- 2026-10-09: Decisions D-12 to D-14 recorded (both sides, separate processes, default intervals). Wireframes rebuilt for approval.
- 2026-10-09: Wireframes approved. Pack handed off for buildout.
- 2026-10-09: D-16 closed: Polymarket US selected (amendment A9). D-18 Wave 1 Fed-pair gate added.
- 2026-10-09: D-06 (venue roles in config) and D-07 (ignore prices after as_of) approved.
- 2026-10-09: D-08 approved (keep the settlement-wording limitation in the README). All decisions closed; pack ready for Wave 0.
- 2026-10-09: T-00 started; scaffolding, CI, guard, and acceptance runner are in progress.
- 2026-10-09: T-00 in review (!1). Scaffold, cobra stubs, Makefile, guard, acceptance runner; make test lint guard pass. Surprises: no prior T-00 work existed on disk or remote, so Claude Code took it over from the Copilot harness; staticcheck 0.8.1 cannot read Go 1.27 export data, so lint pins a staticcheck master commit; repo is on GitLab, so .github/workflows/ci.yml does not run here; -race skipped locally (no cgo on Windows), runs in Linux CI.
- 2026-10-09: T-01 in review (!2, stacked on !1). Card contracts (Opt with empty never equal, Decimal, integer Micros, Op.Complement, Instant), vocab and config loaders, validation helpers, SQLite schema and repositories, cycles, run lock. Surprises: schema extended beyond docs/architecture.md with cycle_id on data rows, card versioning, raw_responses.kind, skips.stage and subject_counts (assumptions A-5 to A-9); modernc sqlite raised go.mod to go 1.26.
- 2026-10-09: T-02 in review (!3, stacked on !2). Fixture layout and manifest format (fixtures/README.md), fixture transport that fails unknown requests, timeout / HTTP 500 / malformed JSON faults, NoNetwork transport, fixed AsOf, temp SQLite helper. Nothing surprising.
- 2026-10-09: Wave 0 done; stopped at the wave boundary for approval.
- 2026-10-10: Owner approved Q-2 (equinox unlock) and Q-3 (.gitlab-ci.yml) and asked to batch changes. Wave 1 batched on branch wave-1, one commit per task.
- 2026-10-10: T-12 matcher and T-13 router in review on wave-1; tests 1-15 pass.
- 2026-10-10: T-10 Kalshi fetcher (KXFED + KXFEDFUNDSYEAR) and T-11 Polymarket US fetcher in review on wave-1. Real Fed fixtures recorded 2026-10-10T02:25Z.
- 2026-10-10: D-18 gate FAILED. Polymarket US has Fed thresholds only for year end 2027, worded "greater than or equal to X%" (gte); Kalshi words every Fed market "above X%" (gt) and lists no lt. gt X vs gte X is not_same (test 5), and no exact complement exists. unclear_result is also set differently (Kalshi empty, Polymarket yes_or_no_only). Stopped for owner decision.
- 2026-10-10: Owner chose D-19 (ship the honest result plus a labelled synthetic pair) and D-20 (unclear_result only from contract terms). Kalshi KXFED now yes_or_no_only citing FED.pdf Rule 100.5; KXFEDFUNDSYEAR stays empty because ECONSTAT.pdf makes the payout discretionary (found while verifying); Polymarket US empty. Gate passes.
