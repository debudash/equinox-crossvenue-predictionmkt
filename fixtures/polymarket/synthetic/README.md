# SYNTHETIC Polymarket US fixture (not recorded)

Hand-built for required tests 16 and 19 and the routing demo, per decision D-19. It is not a venue recording. No real Polymarket US Fed market pairs exactly with a real Kalshi market (see `fixtures/kalshi/synthetic/README.md`).

Built in the shape of the real 2026-10-10 Polymarket US recording (`fixtures/polymarket/fed/`, `fixtures/polymarket/malformed/`):
- `markets_0.json`: two markets in the macro category whose rules read the upper bound of the target range for the federal funds rate on December 9, 2026 at 2:00 PM ET, sourced from the United States Federal Reserve, with the explicit sentence "If the outcome cannot be determined, this market resolves to No." (D-20).
  - `synthetic-fedfnds-2026-12-09-gt3pt75`: "greater than 3.75%". Same bet as Kalshi `KXFED-26DEC-T3.75` (byte-equal payoff fields).
  - `synthetic-fedfnds-2026-12-09-lte3pt75`: "less than or equal to 3.75%". Opposite of Kalshi `KXFED-26DEC-T3.75`.
- `books/`: order books with hand-set levels. gt 3.75: best offer 0.95, best bid 0.93 (yes 0.95, no 0.07). lte 3.75: best offer 0.06, best bid 0.03 (yes 0.06, no 0.97).

Every market question and rules text says "synthetic" so it cannot be mistaken for venue data in the feed.
