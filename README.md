# Equinox - cross-venue prediction market comparison and routing simulation

A feasibility simulation for PEAK6 Labs. It fetches Kalshi and Polymarket US markets (both CFTC-regulated exchanges), stores each as one market card, recognizes `same` and `opposite` bets by exact payoff-field equality, and simulates which venue is cheaper for a position, using saved prices only. It places no orders.

Governing principle: precision over recall. When the system is not sure, it does not route, and it records why.

## Viability verdict
Written in T-30 from measured results only (docs/metrics.md). It will answer:
1. Is cross-venue normalization achievable? (payoff-field fill rates, out-of-vocabulary rate)
2. What are the tradeoffs? (docs/architecture.md, ADRs)
3. How complex is routing? (route actions, refusal reasons, fee coverage)

## Quickstart
```
make setup && make test
equinox fetch
equinox match
equinox route --side yes
equinox serve --addr :8080
equinox watch                 # optional: background refresh, discovery every 30 min, prices every 2 min
```
More in docs/deployment.md.

## How it works
Four pieces run in order and each reads only what earlier pieces saved. Fetchers are the only code that calls a venue. The matcher compares a Kalshi card with a Polymarket card on seven payoff fields (subject, cutoff operator, value, unit, deadline, decider, unclear-result rule) and returns `same`, `opposite` or `not_same` with a reason. The router compares the cost of the same position on both venues from saved prices, refuses on missing or stale prices, and writes a decision with a plain-sentence reason. A read-only feed shows it all from an in-memory snapshot of the latest completed cycle. An optional `watch` process refreshes markets and prices in the background (amendments A1 to A9 in docs/build-requirements.md).

## Vocabulary coverage
V1: Federal Reserve rate markets only (config/vocab.yaml). T-30 records the exact entries and card counts here.

## Fetcher normalization choices
- Kalshi: internal/fetch/kalshi/NORMALIZATION.md
- Polymarket: internal/fetch/polymarket/NORMALIZATION.md

## Known limitations
- Sending orders, wallets, accounts or moving money
- A third venue
- A similarity percentage, embedding distance or language-model verdict
- Treating "close" markets as the same bet (tighter cutoff, earlier deadline, different data source)
- One-to-many matches
- Order size, book depth, and maker versus taker fills
- Spread or arbitrage statistics
- Kalshi multivariate combo markets
- Choosing a route from live calls made during the decision
- Production UI, regulatory or compliance claims
- Settlement mechanism is not a payoff field. Both venues settle centrally, but two cards can be `same` and still differ in rule wording (docs/spec.md section 8)
- Polymarket coverage is Polymarket US. If no matchable Fed pair exists there, the build falls back to Polymarket International and says so here

## Repository map
| Path | What |
|---|---|
| docs/build-requirements.md | Authoritative build requirements, with approved amendments |
| docs/decisions.md | Every decision, who made it, and its status |
| AGENTS.md | Rules for every coding agent |
| docs/architecture.md | HLD and LLD diagrams, tradeoffs |
| docs/adr/ | Decisions with alternatives |
| docs/spec.md | Card model as an RFC-style draft, CDM mapping, path to standardization |
| docs/citations.md | Business sources and technology references |
| docs/metrics.md, evals/ | What is measured, match audit |
| docs/red-team.md | Adversarial cases |
| Demo_Script.md | Timed demo |
| TASKS.md, docs/tasks/ | Agent task board and specs |
| docs/ai-usage-log.md | AI usage disclosure |
| Wireframes | https://claude.ai/artifact/TghfanvCGFAfjUs5bsRk34 (workflow, command line, feed, post; approved) |

## Path to standardization
Venues have absorbed normalization layers rather than adopting neutral ones. The party with the incentive to standardize equivalence is a neutral intermediary that routes across venues. docs/spec.md drafts the card model as an extension of the FINOS Common Domain Model that PEAK6 / Apex could sponsor (ADR-007).

## AI usage
No AI in matching or routing. Development was AI-assisted; see docs/ai-usage-log.md.
