# ADR-008: Scheduled refresh and an in-memory feed cache

Status: Accepted (amendments A1 to A5). Requirements: FR-1, NFR-2, NFR-3.

## Context
Running fetch, match and route by hand leaves the feed stale between runs, and prices go stale after 10 minutes. Scrolling should be fast. The feed must still write nothing and show only saved records.

## Decision
- `equinox watch` runs a discovery loop and a price loop in one process that holds the run lock. Match runs only when a card is added or a payoff field changed (D-21); a display-only change such as 24h volume does not re-run it. Route runs for a pair only when a needed price changed, with `as_of` set to the refresh time.
- Every poll saves a price record even when the price is unchanged, because freshness is judged by `priced_at`.
- On a price change, route runs for both `yes` and `no`, so the feed covers both positions (A6).
- `watch` and `serve` are separate processes with SQLite as the only handoff (A7).
- Defaults: discovery 30 minutes, prices 2 minutes, retention 24 hours; startup refuses a price interval at or above the 10-minute age limit (A8).
- Every pass completes as a numbered cycle. Readers only see completed cycles.
- `equinox serve` keeps an immutable in-memory snapshot of the latest completed cycle, rebuilt from SQLite when a newer cycle completes, swapped atomically. The previous snapshot is kept so an in-progress scroll finishes on its own cycle.
- Retention prunes price records and price raw responses older than 24 hours; card raw responses and the log are kept.

## Options considered
| Option | Why rejected |
|---|---|
| Fetch on scroll or pull-to-refresh | Feed would write, results would shift mid-read, venue failures would surface in page views |
| watch pushes directly into the cache | Cache could hold data not yet in SQLite; breaks traceability and crash recovery |
| Read SQLite on every page view | Correct and simple, but slower and not point-in-time consistent across pages |
| One process (`serve --watch`) | Simpler to run, but a scheduler crash would take the feed down |
| Route one configured side only | Bot replies would cover half the positions |
| Venue streaming feeds for price changes | Real push, much more complexity; polling within the age limit is enough for V1 |

## Consequences
- The feed is at most one cache check interval behind SQLite.
- The CLI keeps working on its own and is refused while `watch` holds the lock.
