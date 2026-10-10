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
| A-5 | Every data row (cards, prices, decision_log, raw_responses, skips, fetch_runs, subject_counts) carries the cycle_id that wrote it. Readers see only rows from completed cycles; a writer also sees its own open cycle. This is how W5 is enforced | Wave 0 review |
| A-6 | Cards are versioned: a new row is written only when a field other than raw_ref and fetched_at changes. Readers take the latest visible version. Rebuild clears cards and re-saves from raw responses in id order, reproducing identical rows | Wave 0 review |
| A-7 | The store stamps decided_at and cycle and run times from an injectable clock, so matcher and router code never read the clock | Wave 0 review |
| A-8 | raw_responses.kind is markets or prices, so retention prunes price responses and keeps market responses for rebuild. storage.raw_dir removed from config because bodies live in SQLite | Wave 0 review |
| A-9 | Card counts per subject are saved on every match run in an append-only subject_counts table. Skips also carry a stage (fetch, rebuild, match, route) so matcher and router skips use the same table | Wave 0 review |

## Open
| ID | Question | Who | Default until answered |
|---|---|---|---|
| Q-1 | Closed 2026-10-09: Polymarket US (D-16) | - | - |
| Q-2 | If watch crashes it leaves the run lock row behind, and every writing command is refused until it is cleared. Add an explicit `equinox unlock`, or accept manual clearing? | Owner | Manual: delete the run_lock row; the refusal message names the holder and time |
| Q-3 | The repo is on GitLab, so .github/workflows/ci.yml (T-00) never runs. Add .gitlab-ci.yml with the same steps? | Owner | No CI runs; make test lint guard is run locally before each MR |
