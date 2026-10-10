# Demo script (target 6 minutes)

Mixed demo (decision D-19): segments 2 and 3 run live on real data and show the refusals; segments 4 and 5 use the SYNTHETIC demo database, which needs no network. Timings below are from the 2026-10-10 dry run.

Two terminals, two configs, both in config/ (vocab.yaml is read from the config's directory):
- `config/equinox.yaml`: real data, `data/equinox.db`
- `config/demo.yaml`: SYNTHETIC demo, `data/demo.db`

| # | Time | Criterion | Command or screen | Say |
|---|---|---|---|---|
| 1 | 0:00-0:40 | Problem framing | README verdict | "PEAK6 asked whether cross-venue normalization is feasible. Here is the measured answer, then the evidence." |
| 2 | 0:40-1:40 | Venue integration | `equinox fetch --config config/equinox.yaml` (about 50 s; start it while talking) | "Two venues, raw responses saved first, then one card per market. Here are the skips, each with a reason, including the venue rate-limiting us." |
| 3 | 1:40-2:40 | Equivalence definition | `equinox match --config config/equinox.yaml` (about 10 s), then the closest pair's reason | "10,054 pairs, all refused. The closest pair agrees on five of seven fields. Kalshi says above 3.25%, Polymarket says 3.25% or above. At exactly 3.25% they pay differently, so it is not the same bet, and the log says so." |
| 4 | 2:40-3:40 | Routing and justification | `go run ./scripts/seed --db data/demo.db`, then `equinox route --side yes --config config/demo.yaml` | "This pair is SYNTHETIC and labelled so. Routing reads saved prices only. Cheaper wins; fees count only when known on both, and here they are unknown, so the reason says so." |
| 5 | 3:40-4:40 | Explainability and freshness | `equinox serve --config config/demo.yaml`; Matched sort; open a post; in the other terminal rerun seed; reload on the banner | "The feed reads a snapshot of the last completed cycle. A new cycle completed, the banner offers a reload. Every badge and reason traces to a saved log record." |
| 6 | 4:40-5:15 | Determinism | Rerun route with the same `--as-of`; `equinox rebuild --config config/equinox.yaml` | "Same saved inputs, same answer. Cards rebuild from raw responses without calling a venue." |
| 7 | 5:15-6:00 | Results | `make acceptance`, docs/metrics.md | "Twenty of twenty, eleven of eleven. And the field that most often separates markets that look alike is the cutoff operator." |

Fallback for segment 2: if a venue API fails, say so and show the other venue's data and every later step still work. That is test 19 live. If both fail, use the database from the dry run.

## Pre-flight
- [ ] `equinox fetch` and `match` on config/equinox.yaml within the hour (dry run: fetch 49 s, match 10 s)
- [ ] `go run ./scripts/seed --db data/demo.db` within 10 minutes of segment 4, or routes will be stale
- [ ] `equinox unlock --config config/equinox.yaml` shows the lock is free
- [ ] Take `--as-of` after the seed or fetch you route on: prices saved after `as_of` are ignored (D-07), so an earlier `as_of` shows them as missing
- [ ] Feed at 1280 wide; terminal font enlarged
