# ADR-004: Failure behavior - isolate, skip, count, continue

Status: Accepted. Requirements: NFR-3, FR-7. Build reference: "Failure behavior".

## Context
Public APIs fail and return inconsistent data. The PRD requires graceful handling.

## Decision
- Each fetcher runs independently with a timeout. If one venue fails, its error is saved and the other venue's data is kept.
- Pairs that need a card or price from a failed venue are `no_route`, with the missing item named. Regulated markets follow the same idea: a trading center may bypass a venue that fails to respond rather than wait on it [Reg NMS 611, self-help].
- Inconsistent records (unknown venue, side other than yes or no, price outside 0 to 1, unparseable deadline) are skipped, counted and logged with a reason. No record disappears silently.
- Matcher and router never retry a venue call because they never make one.

## Options considered
| Option | Why rejected |
|---|---|
| Retry until success | Blocks the run and hides outages |
| Use stale data from a failed venue | Hidden staleness; the 10-minute age limit handles this explicitly |
| Health state machine with probes | Useful in production, unnecessary for a run-in-order simulation |

## Consequences
- Every fetch run leaves saved and skipped counts with reasons, which feed the viability verdict.

## Sources
[Reg NMS 611, self-help]
