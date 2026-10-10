# Assumptions and open questions

## From the build requirements (written in code)
- The payoff fields, not the title, are the bet.
- "The same bet" means the same payoff, not a related event.
- A fee changes the cost. It never changes whether two markets match.
- Kalshi wins an exact cost tie (configured as reference_venue).
- Empty means unknown. Unknown never counts as equal.
- A price older than 10 minutes is unknown.
- Both venues pay 1 dollar per winning contract, treated as equal. With Polymarket US (amendment A9) both settle in USD.
- This build simulates a choice. It does not claim the choice can be executed live.

## Implementation assumptions
| ID | Assumption | Revisit when |
|---|---|---|
| A-1 | Router uses the latest price with priced_at not after as_of, so a rerun at the same as_of ignores later fetches. Approved as D-07 | - |
| A-2 | Kalshi and Polymarket market data endpoints are readable without auth | T-10 / T-11 confirm against official docs |
| A-3 | The Polymarket fetcher reads Polymarket US through its public market data API; prices are integer-encoded strings | T-11 confirms field names and rate limits against docs.polymarket.us |
| A-4 | Fees are read or derived per the venue's published schedule, else stored empty | Official fee docs differ by market type |

## Open
| ID | Question | Who | Default until answered |
|---|---|---|---|
| Q-1 | Closed 2026-10-09: Polymarket US (D-16) | - | - |
