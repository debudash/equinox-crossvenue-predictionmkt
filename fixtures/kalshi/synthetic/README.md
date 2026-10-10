# SYNTHETIC Kalshi fixture (not recorded)

Hand-built for required tests 16 and 19 and the routing demo, per decision D-19. It is not a venue recording.

On 2026-10-10 no real Kalshi and Polymarket US Fed market pair had equal payoff fields: Kalshi words Fed thresholds "above X%" (`gt`), Polymarket US words them "greater than or equal to X%" (`gte`) for year end 2027 only, and real Polymarket US Fed rules carry no fallback sentence, so their `unclear_result` is empty (D-20). The real fixtures in `fixtures/kalshi/fed/` show the matcher refusing that closest pair, naming `cutoff_op`.

What this scenario is:
- `events_kxfed.json`: the real KXFED-26DEC event recorded 2026-10-10 (same JSON shape the fetcher reads), cut to markets `KXFED-26DEC-T3.75` and `KXFED-26DEC-T4.00`. The T3.75 top of book is hand-set (yes ask 0.96, no ask 0.05) so the demo routes both ways. Everything else is as recorded.
- `events_empty.json`: KXFEDFUNDSYEAR returns no events.

`KXFED-26DEC-T3.75` normalizes to: `fed_funds_upper_bound`, `gt`, `3.75`, `percent`, `2026-12-09T19:00:00Z`, `federal_reserve`, `yes_or_no_only`. Its counterpart is `fixtures/polymarket/synthetic/`.
