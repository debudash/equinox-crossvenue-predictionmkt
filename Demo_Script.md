# Demo script (target 6 minutes)

Each segment maps to an evaluation criterion. Fallback for segment 2: if a live API fails, say so and show that the other venue's data and every later step still work. That is test 19 live.

| # | Time | Criterion | Command or screen | Say |
|---|---|---|---|---|
| 1 | 0:00-0:40 | Problem framing | README top | "PEAK6 asked whether cross-venue normalization is feasible. Here is the measured answer, then the evidence." |
| 2 | 0:40-1:30 | Venue integration | `equinox fetch` output | "Two venues, raw responses saved first, then one card per market. Here are the skips, each with a reason." |
| 3 | 1:30-2:30 | Equivalence definition | `equinox match` output, then a not_same record | "Two markets are the same bet only when all seven payoff fields are equal. No score. This pair has the same title but a different deadline, so it is not the same bet, and the log says so." |
| 4 | 2:30-3:30 | Routing and justification | `equinox route --side yes --as-of <time>` | "Routing reads saved prices only. Cheaper wins, fees only when known on both. Here is a refusal for a stale price, recorded like a choice." |
| 5 | 3:30-4:30 | Explainability and freshness | Start `equinox watch`; feed Matched sort; open a post; wait for the new-data banner and reload | "The feed reads a cached snapshot of the last completed cycle, so scrolling is instant. Watch refreshes prices in the background; the banner tells you when a newer cycle is ready. Every badge and reason still traces to a saved log record." |
| 6 | 4:30-5:10 | Determinism | Rerun route with the same as_of; `equinox rebuild` | "Same saved inputs, same answer. Cards rebuild from raw responses without calling a venue." |
| 7 | 5:10-6:00 | Results | `make acceptance`, docs/metrics.md | "Twenty of twenty. And here is which payoff field most often separates markets that look alike." |

## Pre-flight
- [ ] Fetch within the hour; note as_of
- [ ] Identify one same or opposite pair and one same-title not_same pair
- [ ] Identify one stale or missing-price no_route
- [ ] Feed running at 1280 wide; terminal font enlarged
