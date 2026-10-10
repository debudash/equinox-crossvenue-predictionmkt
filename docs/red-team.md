# Red team

Adversarial and failure cases beyond the 20 required tests. Each has a test named by its id (T-22) unless it maps to a required test.

| ID | Case | Expected behavior | Test |
|---|---|---|---|
| R-01 | Same title, different decider | not_same, reason names decider | required 3 |
| R-02 | Same title, deadline one hour apart | not_same, reason names deadline | required 4 |
| R-03 | gt 3.5 vs gte 3.5 | not_same | required 5 |
| R-04 | "3.50" on one venue, "3.5" on the other | Normalized to "3.5"; can match | TestRT_R04 |
| R-05 | Deadline given in a local time zone | Converted to UTC before comparing | TestRT_R05 |
| R-06 | unclear_result differs (last_price vs yes_or_no_only) | not_same, reason names unclear_result | TestRT_R06 |
| R-07 | Both cards have the same field empty | not_same; empty never equals empty | TestRT_R07 |
| R-08 | Decider text not in vocab.yaml | Stored empty, matches nothing | required 17 |
| R-09 | Empty order book side reported as 0 or 1 | buy_price empty, no_route names it | TestRT_R09 |
| R-10 | Outcome tokens listed in reverse order | YES chosen by label | TestRT_R10 |
| R-11 | Price exactly 10 minutes old vs 10 minutes and 1 second | Exactly 10 is current, older is stale | TestRT_R11 |
| R-12 | Price saved after as_of | Ignored for that as_of | TestRT_R12 |
| R-13 | Fee known on one venue only | Buy price only, fee_applied no | required 12 |
| R-14 | Opposite pair routed on the wrong side | Kalshi yes compared with Polymarket no | required 13 |
| R-15 | Exact cost tie | Reference venue wins every run | required 14 |
| R-16 | One venue API returns 500 | Other venue kept; dependent pairs no_route | required 19 |
| R-17 | Malformed JSON for one market | Skipped, counted, logged | required 20 |
| R-18 | Unknown venue value in saved data | Skipped as inconsistent | TestRT_R18 |
| R-19 | Price outside 0 to 1 | Skipped as inconsistent | TestRT_R19 |
| R-20 | Kalshi multivariate combo market | Skipped and counted | TestRT_R20 |
| R-21 | Venue name, clock read or net/http in matcher or router | `make guard` fails CI | guard script test |
| R-22 | Attempt to update or delete a log record | No code path exists | TestRT_R22 |
| R-23 | watch crashes mid-cycle | Cycle marked incomplete; feed keeps showing the last completed cycle | W5 |
| R-24 | New cycle completes while a reader is on page 2 | Load more continues on the reader's cycle; banner offers reload | W6 |
| R-25 | Steady price for 15 minutes | Still fresh, because each poll saves a record | W2 |
| R-26 | prices_every set to 15m with a 10m age limit | watch refuses to start, naming both values | W11 |
| R-29 | watch stopped for an hour | Feed keeps the last completed cycle; sidebar shows watch stopped and last cycle time; prices shown age past 10 minutes and new routes would refuse them | W10 |
| R-27 | Venue returns rate-limit responses | Backoff applied and logged; other venue continues | TestRT_R27 |
| R-28 | CLI route run while watch is running | Refused with a reason; nothing written | W7 |
