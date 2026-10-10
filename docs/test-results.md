# Test results

Recorded 2026-10-10 on Windows 11, Go 1.27.2, CGO off (so `-race` did not run locally; the Linux CI job runs it). No test calls a live venue.

## make acceptance
```
Required tests
  1   pass
  2   pass
  3   pass
  4   pass
  5   pass
  6   pass
  7   pass
  8   pass
  9   pass
  10  pass
  11  pass
  12  pass
  13  pass
  14  pass
  15  pass
  16  pass
  17  pass
  18  pass
  19  pass
  20  pass
  20 of 20 pass

Amendment tests
  W1   pass
  W2   pass
  W3   pass
  W4   pass
  W5   pass
  W6   pass
  W7   pass
  W8   pass
  W9   pass
  W10  pass
  W11  pass
  11 of 11 pass

```

## Full suite (go test ./... -count=1)

128 top-level tests, 128 pass, 0 fail, 0 skipped. `make test lint guard` exits 0 (go vet, staticcheck, guard).

| Package | Tests |
|---|---|
| cmd/equinox | 5 |
| internal/acceptance | 8 |
| internal/card | 12 |
| internal/feed | 11 |
| internal/feed/cache | 1 |
| internal/fetch/kalshi | 11 |
| internal/fetch/polymarket | 12 |
| internal/fetch/registry | 2 |
| internal/match | 11 |
| internal/redteam | 14 |
| internal/route | 10 |
| internal/store | 11 |
| internal/testkit | 7 |
| internal/watch | 9 |
| scripts/acceptance | 2 |
| scripts/guard | 2 |
