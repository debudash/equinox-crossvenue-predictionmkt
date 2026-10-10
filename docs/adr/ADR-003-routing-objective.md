# ADR-003: Routing chooses the lower cost for one contract

Status: Accepted. Requirements: FR-5, FR-6, NFR-1, NFR-5. Build reference: "Routing".

## Context
The PRD does not define a better venue. Best execution regimes do: they weigh price, costs, speed, likelihood of execution, size and nature of the order [MiFID II Art.27, best execution], and for retail orders treat the best result as total consideration, meaning price plus execution costs [FCA COBS 11.2A, total consideration]. The US standard is reasonable diligence [FINRA 5310, best execution].

## Decision
- Route only `same` and `opposite` pairs. For `opposite`, buying YES on the reference venue is compared with buying NO on the counter venue.
- Refuse when either buy price is empty or stale, naming which.
- Cost is buy price plus fee only when fees are known on both venues; otherwise buy price on both, and the log says fees were not applied. Fees materially change comparisons; for example, one public calculator models a Kalshi taker formula of 0.07 x P x (1-P) against zero fees on Polymarket standard markets [Predictions.io, cross-venue gaps]. Fee values are verified against official venue documentation before use.
- Lower cost wins. Exact ties go to the reference venue (Kalshi), so the choice never flips.
- One contract at the saved buy price. No size, depth or slippage.
- Venue roles (`reference_venue`, `counter_venue`) come from config/equinox.yaml, so router code contains no venue names while behaving exactly as the build requirements specify.

## Options considered
| Option | Why rejected |
|---|---|
| Fee-inclusive on one venue, buy price on the other | Not like for like |
| Midpoint or last trade when buy price is missing | Not an executable price; excluded by the build requirements |
| Size-aware book walking | More realistic, out of scope for V1 |
| Hardcoding "Kalshi" in the tie break | Violates the PRD rule against venue logic in routing; config gives identical behavior |

## Consequences
- This is a simplified total consideration test: price and known costs only. Likelihood of execution, size and settlement risk are named limitations.

## Sources
[MiFID II Art.27, best execution] [FCA COBS 11.2A, total consideration] [FINRA 5310, best execution] [Predictions.io, cross-venue gaps]
