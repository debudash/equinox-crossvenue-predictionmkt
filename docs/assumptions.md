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
| A-3 | The Polymarket fetcher reads Polymarket US through its public market data API; prices are decimal dollar strings (superseded, see A-11) | T-11 confirms field names and rate limits against docs.polymarket.us |
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
| A-10 | Kalshi scope: series KXFED and KXFEDFUNDSYEAR via /events with nested markets. KXFED deadline = event strike_date; KXFEDFUNDSYEAR deadline = market close_time. An ask of exactly 0 or 1 is stored empty. Fee empty (schedule based, rounded per order). unclear_result per D-20: KXFED yes_or_no_only (FED.pdf Rule 100.5), KXFEDFUNDSYEAR empty (ECONSTAT.pdf, discretionary) | Contract terms change |
| A-11 | Polymarket US: market_id is the slug; NO buy price = 1 minus best YES bid (one instrument per market); prices are decimal dollar strings, not integer encoded (differs from A-3); deadline from the rules sentence in ET; fee empty (per-fill rounding and tiered rebates); scope is the macro category filtered to "federal funds rate" | D-18 decision |
| A-12 | Polymarket US unclear_result is empty (D-20): no Fed-specific fallback clause found in the market rules, filed contract terms or PMUS Rulebook as of 2026-10-10. The venue's general docs (settle at $1 or $0) are not treated as a contract-specific source. Limitation: `yes_or_no_only` does not record which way an unclear result falls | A Fed clause is found in the PMUS Rulebook or a CFTC filing |
| Q-2 | Closed 2026-10-10: `equinox unlock --holder <name>` added | - | - |
| Q-3 | Closed 2026-10-10: .gitlab-ci.yml added | - | - |
| Q-4 | Closed 2026-10-10: D-19 (ship the honest result plus a labelled synthetic pair) and D-20 (unclear_result from contract terms only) | - | - |
| Q-5 | Where is the PMUS Rulebook text that governs a Fed contract whose outcome cannot be determined? | Owner or T-30 | Polymarket US unclear_result stays empty |
