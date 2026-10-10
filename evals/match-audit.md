# Match audit

Session: real fetch 2026-10-10T03:41:36Z · match cycle 2 · as_of 2026-10-10T03:42:32Z · auditor: Claude Code (claude-opus-5-5), reading saved cards and rules_text. Owner review: pending.

## same and opposite pairs
None in the real session (0 of 10,054). Zero false matches. The SYNTHETIC pair (decision D-19) is hand-built and so is not audited here; its fields are asserted by test 16.

## not_same sample (10 of 10,054)
| Pair (kalshi / polymarket) | Logged reason | Rules read on both | Reason correct? | Notes |
|---|---|---|---|---|
| KXFEDFUNDSYEAR-28JAN01-T3.25 / rvcus-fedfnds-ondt-2027-12-31-gte3pt25 | cutoff_op differs: gt vs gte | Yes | Yes | Kalshi: "is above 3.25%". Polymarket US: "greater than or equal to 3.25%". At exactly 3.25% Kalshi pays No and Polymarket pays Yes. Also: Kalshi adds "If the Federal Reserve specifies a single target rate rather than a target range, that target rate will be used"; Polymarket US has no such clause. unclear_result is empty on both |
| KXFEDFUNDSYEAR-28JAN01-T3.50 / ...-gte3pt50 | cutoff_op differs: gt vs gte | Yes (same template) | Yes | Same pattern as above |
| KXFEDFUNDSYEAR-28JAN01-T4.00 / ...-gte4pt00 | cutoff_op differs: gt vs gte | Yes (same template) | Yes | Same pattern |
| KXFEDFUNDSYEAR-28JAN01-T4.50 / ...-gte4pt50 | cutoff_op differs: gt vs gte | Yes (same template) | Yes | Same pattern |
| KXFEDFUNDSYEAR-28JAN01-T5.25 / ...-gte5pt25 | cutoff_op differs: gt vs gte | Yes (same template) | Yes | Same pattern |
| KXFED-26DEC-T3.75 / rdc-usfed-fomc-2026-12-09-cut25 | cutoff_op is empty on polymarket | Yes | Yes | Decision bucket ("25 bps decrease"), not a threshold; no cutoff exists to compare |
| KXFED-26DEC-T3.75 / rdc-usfed-fomc-2026-12-09-cut50 | cutoff_op is empty on polymarket | Yes | Yes | Decision bucket |
| KXFED-26DEC-T3.75 / rdc-usfed-fomc-2026-12-09-hike25 | cutoff_op is empty on polymarket | Yes | Yes | Decision bucket |
| KXFED-26DEC-T3.75 / rdc-usfed-fomc-2026-12-09-nochng | cutoff_op is empty on polymarket | Yes | Yes | "does not change the upper bound"; also has a rounding rule for changes between displayed options, which Kalshi does not have |
| KXFED-26DEC-T3.75 / rvcus-fedfnds-ondt-2027-12-31-gte3pt75 | cutoff_op differs: gt vs gte | Yes | Yes | Also a different deadline (Dec 9, 2026 meeting vs Dec 31, 2027); the reason names the first differing field in canonical order |

Disagreements found: none. Every reason names a field that does differ or is empty. Fetcher bugs found: none.

## Routing check
Real session: no route records (no matched pairs). SYNTHETIC pair, recomputed by hand from the logged prices:
| Record | Logged | By hand |
|---|---|---|
| same, side yes | kalshi yes 0.96 vs polymarket yes 0.95: buy_on_polymarket | 0.95 < 0.96, correct |
| opposite, side yes | kalshi yes 0.96 vs polymarket no 0.97: buy_on_kalshi | 0.96 < 0.97, correct |
| same, side no | kalshi no 0.05 vs polymarket no 0.07: buy_on_kalshi | correct |
| opposite, side no | kalshi no 0.05 vs polymarket yes 0.06: buy_on_kalshi | correct |
All four say fees were not applied because fees are unknown on both venues.
