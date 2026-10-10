# Metrics

Measured in T-30 from one real fetch, match and route session plus the test suite. No targets are invented ahead of data except where the build requirements fix them.

## Correctness (fixed by the build requirements)
| Metric | Target | Source |
|---|---|---|
| Required tests passing | 20 of 20 | `make acceptance` |
| Determinism: identical action and reason on rerun | 100% | test 15 |
| Rebuild produces identical cards | 100% | test 18 |
| False matches found in the match audit | 0 | evals/match-audit.md |

## Normalization (answers "is it achievable?")
| Metric | Definition |
|---|---|
| Cards per venue | Total saved, per category |
| Fed cards with all payoff fields filled | Share per venue, and which field is most often empty |
| Skips by reason | Counted per venue per run |
| Out-of-vocabulary rate | Share of cards with an empty subject or decider |

## Equivalence (answers "what blocks it?")
| Metric | Definition |
|---|---|
| Pairs compared | Same-subject pairs |
| same / opposite / not_same counts | From match records |
| not_same by deciding field | Count per field named in the reason. This is the headline evidence: which payoff field most often separates two markets that look alike |

## Routing (answers "how complex is it?")
| Metric | Definition |
|---|---|
| Actions | buy_on_kalshi, buy_on_polymarket, no_route counts |
| no_route by reason | not same, missing price, stale price |
| fee_applied rate | Share of route records with fees applied on both venues |
| Ties | Count of decisions settled by the tie break |

## Background refresh and feed (amendments)
| Metric | Definition |
|---|---|
| Cycle duration | Discovery and price cycles, median and max |
| Cycle failures | Count per venue, with reasons |
| Price freshness at route time | Age of prices used, median and max; must stay under 10 minutes |
| Feed page render time | Server time to render a page from the cache, p95, measured with a Go benchmark |
| Cache lag | Time from cycle completed to snapshot swapped |
