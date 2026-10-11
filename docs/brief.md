# Equinox brief

Sources: the PEAK6 "Project Equinox" overview and PRD (v1, 8/7/2026), and docs/build-requirements.md (final, Oct 9, 2026), which is authoritative for the build. Technical contact: Bryce Harris.

## The question PEAK6 is asking
1. Is cross-venue normalization realistically achievable?
2. What architectural tradeoffs exist?
3. How complex does routing become in practice?

The code is evidence. The README verdict answers these from measured results only (docs/metrics.md).

## PRD requirements and how the build meets them
| ID | PRD requirement | How the build meets it |
|---|---|---|
| FR-1 | Fetch live market data from two venues | Kalshi and Polymarket fetchers, raw responses saved |
| FR-2 | Internal representation of a market | One market card: payoff fields decide, record fields display |
| FR-3 | Identify potentially equivalent markets; define equivalence | `same` / `opposite` / `not_same` by exact payoff-field equality (ADR-001) |
| FR-4 | Normalized price comparison | Price records per side in dollars 0 to 1, fees when known on both venues |
| FR-5 | Simulate a routing decision | Router on saved data only, cheaper venue wins, fixed tie break (ADR-003) |
| FR-6 | Log reasoning for decisions | Append-only decision log with plain-sentence reasons for matches, refusals and routes |
| FR-7 | Handle imperfect data | Empty means unknown; skip, count and log malformed records (ADR-004) |
| NFR-1 | Deterministic routing | `as_of` passed in, saved inputs only, fixed tie break (test 15) |
| NFR-2 | No blocking external calls during routing | Router reads SQLite only (`make guard`) |
| NFR-3 | Graceful API failure | One venue failing never stops the other or downstream steps (test 19) |
| NFR-4 | Separation of concerns | Fetchers, matcher, router, feed as separate pieces reading only saved data |
| NFR-5 | No hardcoded venue logic in routing | Venue roles from config; guard forbids venue names in matcher and router |
| NFR-6 | Reasonable test coverage | 20 required tests plus red-team tests |
| NFR-7 | Go, GCP preferred, local acceptable | Go + SQLite; runs locally and as one Docker image, hosted on Railway (ADR-009 records why not GCP: `watch` needs an always-on process over a persistent SQLite file) |
| NFR-8 | Reusable foundation for more venues | New venue = new fetcher + vocabulary entries; matcher and router unchanged |
| NFR-9 | AI usage documented | No AI in the core's match or route (ADR-006); development AI usage logged (docs/ai-usage-log.md). The added-scope proposal in prototype/ calls an LLM outside the core, behind a human decision |

## Deliverables
| Deliverable | Where |
|---|---|
| Source code | repo |
| Technical documentation | docs/architecture.md, docs/spec.md, docs/adr/, fetcher NORMALIZATION.md files |
| Demo video | https://youtu.be/egQVWj9Dtno; scene list in Demo_Script.md |
| Proposal prototype (added scope) | prototype/README.md; live at /app |
| Test results | `make acceptance` output in docs/test-results.md, docs/metrics.md |
| AI usage log | docs/ai-usage-log.md |
| Deployment guide | README.md, docs/deployment.md |

## Key decisions
| Topic | Decision | Record |
|---|---|---|
| Equivalence | Exact payoff-field equality; `opposite` only for exact complementary cutoffs | ADR-001 |
| Determinism | Saved data only, `as_of` passed in, raw responses saved, `rebuild` | ADR-002 |
| Routing objective | Lower cost for one contract; fees only when known on both; reference venue wins ties | ADR-003 |
| Failure | Isolate venues; skip, count, log; never retry from matcher or router | ADR-004 |
| Normalization | Shared vocabulary; out-of-vocabulary values stored as empty | ADR-005 |
| AI | No model in match or route | ADR-006 |
| Standards | Spec written as a draft that extends FINOS CDM; no patent | ADR-007 |
| Freshness and speed | Background watch; feed served from an in-memory snapshot of the latest completed cycle | ADR-008 |
| Hosting | Railway, one image, persistent volume; the proposal app shares the service at /app | ADR-009 |

## Scope
V1 vocabulary covers Federal Reserve rate markets only. Out of scope items are listed in docs/build-requirements.md and repeated in the README.
