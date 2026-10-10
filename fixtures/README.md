# Fixtures

Saved raw venue responses. Tests never call a live venue (AGENTS.md); they serve these files through `internal/testkit`.

## Layout
```
fixtures/<venue>/<scenario>/
  manifest.json      which file answers which request
  <response files>   raw bodies exactly as the venue returned them
```
Scenarios in this repo:
| Scenario | Kind |
|---|---|
| `kalshi/fed`, `polymarket/fed` | Real, recorded 2026-10-10. No exact cross-venue pair exists in them (decision D-19) |
| `kalshi/malformed`, `polymarket/malformed` | Real records, hand-edited to break one record each |
| `kalshi/synthetic`, `polymarket/synthetic` | SYNTHETIC, hand-built for tests 16 and 19 and the demo: one same pair and one opposite pair |
| `faults/polymarket/counterpart_27jan`, `faults/polymarket/oov_decider` | SYNTHETIC one-field variants for test 17 |
| `*/http_500`, `*/malformed_json`, `kalshi/timeout`, `polymarket/rate_limited` | Fault scenarios |

Each venue directory is owned by its fetcher task (T-10 owns `fixtures/kalshi/**`, T-11 owns `fixtures/polymarket/**`, T-22 owns `fixtures/faults/**`). Response files are saved byte-for-byte; do not reformat them. The repo keeps line endings as committed (`core.autocrlf=false` locally), so hashes match on every machine.

## manifest.json
Illustrative example (the real Kalshi fetcher calls `/events?series_ticker=...&with_nested_markets=true`; see the recorded manifests):
```json
{
  "description": "One real December 2026 Fed market, recorded 2026-10-10.",
  "routes": [
    {"path": "/trade-api/v2/markets", "query": "status=open&series_ticker=KXFED", "file": "markets.json"},
    {"path": "/trade-api/v2/markets/KXFED-26DEC-T3.50/orderbook", "file": "orderbook.json"},
    {"path": "/trade-api/v2/events", "fault": "http_500"}
  ]
}
```
| Field | Meaning |
|---|---|
| method | Default `GET` |
| path | Request path. The host is ignored, so `base_url` in config does not matter |
| query | Matched as a set of parameters; order does not matter. Omit for none |
| file | Response body, relative to the scenario directory. Required unless `fault` is set |
| status | Default 200 |
| fault | `timeout` (hangs until the request context ends, then fails as a timeout), `http_500`, or `malformed_json` (serves `file` cut in half, or `{` with no file) |

Any request not listed fails with `testkit.ErrUnknownRequest`, so a fetcher that asks for an unrecorded page fails the test instead of reaching the network.

## Using it in a test
```go
tr := testkit.Scenario(t, testkit.Fixture(t, "kalshi", "fed"))
f := kalshi.New(cfg, vocab, tr.Client(), testkit.FixedClock(testkit.AsOf))   // an *http.Client and a clock
st := testkit.TempStore(t)                 // fresh SQLite, clock fixed at testkit.AsOf
```
- `testkit.AsOf` is the fixed `as_of` for every test.
- `testkit.NoNetwork` refuses every request; use it to prove `rebuild`, `match` and `route` make no venue call.
- `testkit.FixedClock(testkit.AsOf)` gives fetchers and the store a deterministic clock.

## Recording a fixture
Save the venue's response body unchanged (for example `curl -s '<url>' -o markets.json`), add a route for the exact path and query the fetcher sends, and note the recording date in `description`. Record only public market data endpoints; never save credentials or account data.
