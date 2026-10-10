# ADR-002: Match and route on saved data only

Status: Accepted. Requirements: NFR-1, NFR-2, FR-1. Build reference: locked choices 5 and 9, "System shape".

## Context
The PRD asks for live data, deterministic routing, and routing that never blocks on external calls. Calling venues inside routing violates the last two.

## Decision
- Fetchers are the only network-facing code. They save every raw response before parsing.
- Matcher reads cards only. Router reads match records and price records only, plus `as_of` passed in. Neither reads the clock or the network.
- The router uses the latest price with `priced_at` not after `as_of`, and treats prices older than 10 minutes before `as_of` as empty.
- `equinox rebuild` re-runs normalization over saved raw responses without calling a venue.
- SQLite (pure Go driver) holds raw responses, cards, prices, fetch runs, skips and the append-only decision log.
- Prices and fees are parsed into integer micro-dollars. No floats.

## Options considered
| Option | Why rejected |
|---|---|
| Fetch inside route | Blocks on network, not reproducible |
| JSON files instead of SQLite | Fine for snapshots, weaker for latest-price-by-time queries and the feed |
| Background cache without raw responses | Loses rebuild and audit |

## Consequences
- Same saved inputs and `as_of` always give the same action and reason (test 15).
- A decision can use a price up to 10 minutes old. The log records the price time used.
