# Demo script

## Recorded video (2:29, 2026-10-11)

Watch: https://youtu.be/egQVWj9Dtno

Narrated walkthrough recorded from a local copy of the deployed build (same code and core data as Railway; the proposal app wrote to its own database, reset before the take). App footage is real; code and log panels are cut from the repo files and the synthetic demo's `decision_log`. Labels on screen: SYNTHETIC PAIR, SIMULATED (no orders), ADDED SCOPE (the proposal).

| Time | Scene | Shows |
|---|---|---|
| 0:00 | Equinox: the problem and three challenges | "This is Equinox." One Fed question at two prices (NO: 0.15 on Kalshi, 0.99 on Polymarket); one market language, never pay for the wrong bet, always the lower price |
| 0:13 | How it was built | The agent build factory (requirements, AGENTS.md rules, task files in gated waves); work in progress |
| 0:21 | Internal representation | `card.Payoff`: seven fields decide the bet; record fields are display only |
| 0:31 | Finding equivalent markets | `match.Compare`: exact equality, no score, no model; zero wrong bets at the cost of some missed ones |
| 0:43 | The live result | 0 of 10,054 live pairs match; at exactly 4.00%, "above" and "or above" pay differently |
| 0:54 | A simulated route, logged | To show routing, run on synthetic data (labelled): Polymarket 0.95 beats Kalshi 0.96; the append-only log with plain reasons |
| 1:05 | Deterministic routing | Smart routing can be done deterministically: `as_of` passed in, saved inputs, reference venue wins only exact ties (test 15); guard keeps venue names out of match and route |
| 1:18 | The proposal | "above 3.75%" = "4.00% or above" in 0.25 steps; prototype outside the core |
| 1:32 | Venue policy, the model reads | Nightly versioned policy snapshot; every value quoted word for word |
| 1:43 | Rules measure, human decides, router routes | 9 near misses; a decision with a reason under policy 2026-10-11.1; Buy NO on Kalshi, logged once |
| 1:52 | The learning loop | Each review becomes a few-shot example and an eval case; the human gate stays |
| 2:01 | The answers and the business calls | Do we trust the quarter-point grid? Is venue discretion an acceptable risk? Should matching follow outcomes, not wording? Live URL |

Code-quality evidence is tagged on screen as it appears: separation of concerns, documented assumptions (A-10, A-11), readable code, testing (tests 1-7, test 15, 20 of 20 and 11 of 11), no venue logic in routing (`make guard`).

## Live demo (target 6 minutes)

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
