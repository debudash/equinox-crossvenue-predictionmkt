# Traceability

## Required tests to tasks
| Test | Requirement | Task |
|---|---|---|
| 1 | Equal payoff fields are same despite different titles | T-12 |
| 2 | gt 3.5 vs lte 3.5 is opposite | T-12 |
| 3 | Different deciders is not_same, reason names decider | T-12 |
| 4 | Different deadlines is not_same, reason names deadline | T-12 |
| 5 | gt 3.5 vs gte 3.5 is not_same | T-12 |
| 6 | Empty deadline is not_same, reason names deadline as empty | T-12 |
| 7 | A vs B equals B vs A | T-12 |
| 8 | not_same produces no_route | T-13 |
| 9 | Missing buy price produces no_route | T-13 |
| 10 | Stale price produces no_route, "price is stale" | T-13 |
| 11 | Lower cost wins; fees can change the winner | T-13 |
| 12 | Fee known on one venue only: buy price only, fee_applied no | T-13 |
| 13 | Opposite pair: Kalshi yes vs Polymarket no | T-13 |
| 14 | Equal costs choose Kalshi | T-13 |
| 15 | Same inputs and as_of give same action and reason | T-13 |
| 16 | Same Fed market from both fixtures gives byte-equal payoff fields | T-10, T-11 fixtures; T-22 test |
| 17 | Out-of-vocabulary value stored empty and matches nothing | T-01, T-10, T-11, T-12; T-22 end to end |
| 18 | rebuild reproduces identical cards | T-20 |
| 19 | One venue failing keeps the other; dependent pairs no_route | T-22 |
| 20 | Malformed record skipped, counted, logged; run completes | T-10, T-11; T-22 end to end |

## Amendment tests to tasks
| Test | Requirement | Task |
|---|---|---|
| W1 | Match only after a discovery cycle that changed cards | T-23 |
| W2 | Unchanged price still saved each poll | T-23 |
| W3 | Changed price re-routes that pair only, as_of = refresh time | T-23 |
| W4 | Venue failure in one cycle; next cycle normal | T-23 |
| W5 | Cache shows only fully completed cycles | T-21 |
| W6 | Load more pages within the reader's cycle | T-21 |
| W7 | CLI refused while watch holds the run lock | T-20 |
| W8 | Pruning removes only old price data | T-23 |
| W9 | Price change routes both yes and no | T-23 |
| W10 | serve keeps the last completed cycle when watch is down | T-21 |
| W11 | watch refuses a price interval at or above the age limit | T-23 |

## PRD requirements to evidence
| PRD | Evidence |
|---|---|
| FR-1 two venues | Fetchers T-10, T-11; demo segment 2 |
| FR-2 internal representation | Card model, ADR-005, test 16 |
| FR-3 equivalence | ADR-001, tests 1-7, match audit |
| FR-4 normalized pricing | Price records, test 11-13 |
| FR-5 routing simulation | ADR-003, tests 8-15 |
| FR-6 reasoning logged | Decision log, feed bot replies |
| FR-7 imperfect data | ADR-004, tests 17, 19, 20, red team |
| NFR-1 deterministic | Test 15, test 18 |
| NFR-2 non-blocking routing | `make guard` |
| NFR-3 API failure | Test 19 |
| NFR-5 no venue logic in routing | `make guard`, config roles |
| NFR-6 coverage | `make acceptance`, red-team suite |
