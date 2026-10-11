# Equinox - cross-venue prediction market comparison and routing simulation

A feasibility simulation for PEAK6 Labs. It fetches Kalshi and Polymarket US markets (both CFTC-regulated exchanges), stores each as one market card, recognizes `same` and `opposite` bets by exact payoff-field equality, and simulates which venue is cheaper for a position, using saved prices only. It places no orders.

Governing principle: precision over recall. When the system is not sure, it does not route, and it records why.

## Viability verdict
Measured on 2026-10-10 (docs/metrics.md, docs/test-results.md, evals/match-audit.md).

**Cross-venue normalization works mechanically; on that day no real Fed market pair was the same bet.**
- Both venues normalize into one card: 457 Kalshi and 22 Polymarket US Fed cards, 100% with subject and decider, 0 out of vocabulary. `equinox rebuild` reproduced all 479 card versions from saved raw responses with no venue call.
- The matcher compared 10,054 same-subject pairs and refused all of them, each with the field that differed. Zero false matches.
- The closest real pairs (9) agree on 5 of 7 payoff fields: same subject, threshold, unit, deadline (Dec 31, 2027, 11:59 PM ET) and decider. They differ on the cutoff operator, Kalshi "above X%" (`gt`) versus Polymarket US "greater than or equal to X%" (`gte`), which pays differently at exactly X%. Neither venue states a contract-specific fallback for those markets, so `unclear_result` is unknown on both.
- Routing is shown on one clearly labelled SYNTHETIC pair (decision D-19): cheaper venue chosen per side, fees not applied because they are unknown on both venues, every reason a plain sentence.

Answers to the three questions:
1. **Is normalization achievable?** Yes for the structured fields; the binding constraint is that venues word the same economic question differently, and exact equality (correctly) treats different wording at the boundary as a different bet.
2. **What are the tradeoffs?** Exact equality gives zero wrong-bet routing and a reason for every refusal, at the cost of recall. Recall grows only by adding vocabulary and fetcher rules, or by venues listing exactly complementary contracts.
3. **How complex is routing?** Simple once a pair matches: one comparison of saved executable asks, fees only when known on both venues, a fixed tie break. The hard part is equivalence, not routing.

## Live
https://equinox-production-6620.up.railway.app (Railway, real Kalshi and Polymarket US Fed markets, refreshed by `watch`; read-only). Deployment notes: docs/deployment.md.

The proposal app (core results, live feed, near-miss proposal and a guided test) is at https://equinox-production-6620.up.railway.app/app/ behind a sign-in (username `peak6`; the password is shared separately). Demo video: a 2:25 walkthrough, scene list in Demo_Script.md.

## Proposal prototype (added scope)
The exact matcher is right to refuse, but on live data it finds none of the bets the venues write differently (Kalshi "above 3.75%" is Polymarket "4.00% or above" when rates move in 0.25 steps). `prototype/` is a proposal, outside the core and never imported by it: an LLM reads each market's rules and the venues' versioned policy clauses into the same seven fields, quoting every value; deterministic rules measure the differences (9 near misses of 10,054); a person decides with a reason; approved pairs go through a port of the core's router, checked against the core's own route records. Reviews feed a learning loop and an eval set; the human gate stays. The core keeps no language model (ADR-006). See prototype/README.md.

## Quickstart
```
make setup && make test
go build -o equinox ./cmd/equinox       # equinox.exe on Windows
./equinox fetch                          # live public APIs, no keys
./equinox match
./equinox route --side yes               # --as-of defaults to now
./equinox serve --addr :8080             # read-only feed, open http://localhost:8080
./equinox watch                          # optional, separate terminal: background refresh
./equinox rebuild                        # cards from saved raw responses, no venue call
./equinox unlock                         # shows or clears a run lock left by a crashed writer
go run ./scripts/seed --db data/demo.db  # SYNTHETIC demo database, no venue call
```
`make acceptance` prints the 20 required and 11 amendment tests. More in docs/deployment.md.

## How it works
Four pieces run in order and each reads only what earlier pieces saved. Fetchers are the only code that calls a venue, and they save every raw response before parsing. The matcher compares a Kalshi card with a Polymarket card on seven payoff fields (subject, cutoff operator, value, unit, deadline, decider, unclear-result rule) and returns `same`, `opposite` or `not_same` with a reason. The router compares the cost of the same position on both venues from saved prices, refuses on missing, stale or inconsistent prices, and writes a decision with a plain-sentence reason. A read-only feed shows it all from an in-memory snapshot of the latest completed cycle: an overview with summary cards and one module per category (top 5 posts, every venue represented), Yes prices in green and No prices in red, crosspost badges and the decision log reasons on each post. Besides the Fed markets that can match, both fetchers save display-only markets from a few more real categories (Economics, Politics, Crypto, Financials, Climate and Weather, Sports); those are never matched. `watch` refreshes markets and prices in the background as its own process; SQLite is the only handoff, and one writer runs at a time.

## Vocabulary coverage
config/vocab.yaml, V1: Federal Reserve rate markets only.
| Entry | Values |
|---|---|
| subjects | `fed_funds_upper_bound` |
| deciders | `federal_reserve` |
| cutoff_units | `percent`, `usd`, `count` |

Fetch scope: Kalshi series KXFED (per FOMC meeting) and KXFEDFUNDSYEAR (year end); Polymarket US macro markets whose rules mention the federal funds rate.

Display-only categories fetched, never matched: Kalshi Economics (non-Fed: CPI, gas, jobs, GDP), Politics, Crypto, Financials, Climate and Weather and Sports; Polymarket US politics, crypto, finance, climate and sports. They are listed in config/equinox.yaml under `fetchers.<venue>.display_categories` (bounded per category) and fill the feed with real markets and prices, but their cards keep subject, cutoff, decider and unclear_result empty, so the matcher never pairs them. No vocabulary entry was added.

## Fetcher normalization choices
Every choice is listed next to the code and in:
- Kalshi: internal/fetch/kalshi/NORMALIZATION.md
- Polymarket US: internal/fetch/polymarket/NORMALIZATION.md

Key choices: deadline is the moment the outcome is read (Kalshi KXFED event strike_date, KXFEDFUNDSYEAR close_time; Polymarket US the time stated in the rules, converted from ET); buy prices are executable asks (Polymarket US NO = 1 minus best YES bid), an empty book side is empty, never 0 or 1; fees are empty on both venues because neither is exactly derivable per contract; `unclear_result` is filled only from a source tied to that contract (decision D-20).

## Known limitations
- Sending orders, wallets, accounts or moving money
- A third venue
- A similarity percentage, embedding distance or language-model verdict
- Treating "close" markets as the same bet (tighter cutoff, earlier deadline, different data source). "Above 3.00%" and "3.25% or above" are equal only if rates move in 25 bp steps; the system does not infer that
- One-to-many matches
- Order size, book depth, and maker versus taker fills
- Spread or arbitrage statistics
- Kalshi multivariate combo markets (skipped and counted)
- Choosing a route from live calls made during the decision
- Production UI, regulatory or compliance claims
- No real exact Fed pair existed on 2026-10-10; routing is demonstrated on a labelled SYNTHETIC pair (D-19). The real-data result is the refusal evidence above
- `unclear_result` (D-20): Kalshi KXFED is `yes_or_no_only` from its filed terms (FED.pdf Rule 100.5, no data resolves to No); KXFEDFUNDSYEAR is empty because its terms (ECONSTAT.pdf) make the payout discretionary; Polymarket US is empty because no Fed-specific fallback was found in its rules, filed terms or Rulebook (open question Q-5). `yes_or_no_only` also does not record which way an unclear result falls
- Settlement wording beyond the seven fields can still differ (for example Kalshi's clause for a single target rate instead of a range). Both venues settle centrally (D-08)
- Fees are unknown on both venues, so every route compares buy price only, and says so
- Polymarket US rate-limits order book requests (HTTP 429, Cloudflare). Rate-limited books are skipped and logged; the Kalshi fetcher has no rate-limit backoff
- Each match run writes one log record per compared pair (10,054 for Fed); the log is never pruned
- GitLab CI is switched off (the project has no runner); checks run locally with `make test lint guard`, and the race detector with `make race` (Linux container; passed 2026-10-10)
- Polymarket coverage is Polymarket US (A9). The International fallback was not built (D-19)

## Repository map
| Path | What |
|---|---|
| docs/build-requirements.md | Authoritative build requirements, with approved amendments |
| docs/decisions.md | Every decision, who made it, and its status (D-19, D-20 resolve the Wave 1 gate) |
| AGENTS.md | Rules for every coding agent |
| docs/architecture.md | HLD and LLD diagrams, tradeoffs |
| docs/adr/ | Decisions with alternatives |
| docs/spec.md | Card model as an RFC-style draft, CDM mapping, path to standardization |
| docs/citations.md | Business sources and technology references |
| docs/metrics.md, docs/test-results.md, evals/ | Measured results, test output, match audit |
| docs/red-team.md | Adversarial cases, each with a test |
| fixtures/README.md | Recorded and SYNTHETIC fixture scenarios |
| Demo_Script.md | Timed demo |
| TASKS.md, docs/tasks/, docs/status.md | Agent task board, specs, build log |
| docs/ai-usage-log.md | AI usage disclosure |
| Wireframes | See "Wireframes" below |

## Wireframes
| Document | What it shows |
|---|---|
| docs/wireframes-approved-2026-10-09.pdf | The wireframes approved before implementation (workflow, command line, feed, post). Frozen; it records what was planned |
| docs/screens-as-built-2026-10-10.pdf | The feed as built, printed from the running app: real data (no pair matched, closest refusal shown) and the SYNTHETIC pair (badges, routes) |
| https://claude.ai/artifact/TghfanvCGFAfjUs5bsRk34 | The wireframe canvas, updated to the as-built screens. Private to the owner: share it before sending the link to a reviewer |

What changed from the approved wireframes, and why: the filter bar has an Apply button (the feed works without JavaScript); the sidebar distinguishes watch running, stopped and holding the lock without completing cycles; reasons use the real log templates ("Compared yes on kalshi at 0.96 with yes on polymarket at 0.95 ..."; fees are unknown on both venues, D-20); not_same comparisons fold under the closest refusal; the command line prints counts per cycle, with every per-pair reason in the decision log and the feed.

## Path to standardization
Venues have absorbed normalization layers rather than adopting neutral ones. The party with the incentive to standardize equivalence is a neutral intermediary that routes across venues. The measured blocker here, the same economic question listed with different boundary wording, is exactly what a shared contract vocabulary would remove. docs/spec.md drafts the card model as an extension of the FINOS Common Domain Model that PEAK6 / Apex could sponsor (ADR-007).

## AI usage
No AI in matching or routing (ADR-006). Development was AI-assisted; see docs/ai-usage-log.md.
