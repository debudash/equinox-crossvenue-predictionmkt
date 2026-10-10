# Fault fixtures (T-22)

Hand-built scenarios for integration and red-team tests. None is a venue recording. Layout and manifest format are the same as every other scenario (see `fixtures/README.md`): `fixtures/faults/<venue>/<scenario>/manifest.json`.

| Scenario | Used by | What it is |
|---|---|---|
| `polymarket/counterpart_27jan` | TestReq17 | One SYNTHETIC Polymarket US market equal to Kalshi `KXFED-27JAN-T3.75` (`fixtures/kalshi/malformed`) in every payoff field except decider, which is empty on the Kalshi card (settlement source Bank of England, outside vocab.yaml) |
| `polymarket/oov_decider` | TestReq17 | The SYNTHETIC Polymarket pair (`fixtures/polymarket/synthetic`) with the source changed to the Federal Reserve Bank of New York, outside vocab.yaml |

Venue failures (HTTP 500, timeout, malformed JSON, 429) reuse the fetchers' own scenarios under `fixtures/kalshi/` and `fixtures/polymarket/`. Red-team variants (a price of 1.0000, reversed sides, a 3.50 strike) are made in the test from the synthetic scenarios, in a temp directory, so no extra copy is committed.
