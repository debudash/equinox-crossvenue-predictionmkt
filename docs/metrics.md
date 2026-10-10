# Metrics

Measured on 2026-10-10 from one real session against the live public APIs (fetch 03:41:36Z, match, route yes and no at as_of 03:42:32Z, rebuild), plus a `watch` dry run and the test suite. Routing figures come from the SYNTHETIC pair (decision D-19), because the real session produced no matched pair. Nothing below is estimated.

## Correctness
| Metric | Target | Measured | Source |
|---|---|---|---|
| Required tests passing | 20 of 20 | 20 of 20 | `make acceptance`, docs/test-results.md |
| Amendment tests passing | 11 of 11 | 11 of 11 | same |
| Full suite | - | 128 of 128 top-level tests pass | docs/test-results.md |
| Determinism: identical action and reason on rerun | 100% | 100% | test 15 |
| Rebuild produces identical cards | 100% | 479 of 479 card versions identical, no venue called | test 18; live `equinox rebuild` |
| False matches in the match audit | 0 | 0 (no real pair was matched) | evals/match-audit.md |

## Normalization: is it achievable?
| Metric | Kalshi | Polymarket US |
|---|---|---|
| Fed cards saved (scope: Fed rate markets only) | 457 (series KXFED 247, KXFEDFUNDSYEAR 210) | 22 |
| subject filled | 457 (100%) | 22 (100%) |
| decider filled | 457 (100%) | 22 (100%) |
| cutoff_op, cutoff_value, cutoff_unit filled | 457 (100%), all `gt` | 9 (41%), all `gte`; the other 13 are decision buckets ("No change", "25 bps cut") with no threshold, stored empty by design |
| deadline filled | 457 (100%) | 12 (55%); FOMC bucket rules name a meeting but no time |
| unclear_result filled | 247 (54%): KXFED from FED.pdf Rule 100.5; KXFEDFUNDSYEAR empty because its terms make the payout discretionary (D-20) | 0 (0%): no Fed-specific fallback in the rules or filed terms (D-20) |
| All seven payoff fields filled | 247 (54%) | 0 (0%) |
| Out-of-vocabulary (empty subject or decider) | 0 | 0 |
| Skips | 0 | 189: 185 macro markets outside V1 scope (3 summary rows) and 4 order books refused with HTTP 429 (Cloudflare rate limit); each logged with a reason |
| Fetch time | 0.3 s | 46.2 s (request pacing and 429 backoff) |

## Equivalence: what blocks it?
| Metric | Measured |
|---|---|
| Pairs compared (same subject) | 10,054 (457 x 22) |
| same / opposite / not_same | 0 / 0 / 10,054 |
| not_same by the field named in the reason (first field in canonical order) | cutoff_op empty 5,941; cutoff_op differs 4,113 |
| All blocking fields per pair (every field, not just the first) | unclear_result empty 10,054; cutoff_op, cutoff_value and cutoff_unit empty 5,941 each; deadline differs 5,295; deadline empty 4,570; cutoff_op differs 4,113; cutoff_value differs 3,924 |
| Closest real pairs | 9 pairs equal on 5 of 7 fields: Kalshi KXFEDFUNDSYEAR-28JAN01-T{3.25..5.25} against Polymarket US rvcus-fedfnds-ondt-2027-12-31-gte{3pt25..5pt25}. Same subject, value, unit, deadline (2028-01-01T04:59:00Z) and decider. Separated by cutoff_op (`gt` "above X%" vs `gte` "X% or above") and unclear_result (unknown on both) |

Headline: the payoff field that most often separates markets that look alike is the cutoff operator. Venues word thresholds differently ("above" vs "at or above"), and that is a real payoff difference: at exactly X% one pays and the other does not.

## Routing: how complex is it? (SYNTHETIC pair, D-19)
| Metric | Measured |
|---|---|
| Pairs | 1 same, 1 opposite, 2 not_same (`go run ./scripts/seed`) |
| Actions (yes and no, 4 route records) | buy_on_kalshi 3, buy_on_polymarket 1, no_route 0 |
| fee_applied | 0 of 4. Fees are unknown on both venues (Kalshi fee is schedule based and rounded per order; Polymarket US has per-fill rounding and tiered rebates), so cost is buy price only, and every reason says so |
| Ties settled by the reference venue | 0 |
| Refusals on real data | All 10,054 pairs refused at match; no route was attempted |
| no_route paths exercised | not_same, missing price, stale price, price after as_of, price outside 0 to 1: tests 8 to 10, 19, red-team R11, R12, R19 |

## Background refresh and feed
| Metric | Measured |
|---|---|
| Discovery cycle (watch, live) | 43 s, of which match of 10,054 pairs 10 s |
| Price cycle (watch, live) | under 1 s; no matched real pairs to refresh |
| Match run (CLI) | 9.8 s for 10,054 pairs (one log record each) |
| Cycle failures | 0 in the session; 4 Polymarket US order books rate-limited, recorded as skips, cycle completed |
| Price freshness at route time | Synthetic routes used prices 0 s old; the router refuses anything over 10 minutes (test 10) |
| Feed page render (server, real database, 479 cards) | median 17 ms, max 29 ms over 10 requests |
| Cache lag | at most cache_check_every (5 s) after a cycle completes; not separately timed |
