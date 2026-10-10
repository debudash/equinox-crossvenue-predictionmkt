# Polymarket fetcher: normalization choices

Source: **Polymarket US**, the CFTC-regulated exchange, through its public market data API at `https://gateway.polymarket.us` (no API key). Docs: https://docs.polymarket.us/api-reference/introduction. The card `venue` value is `polymarket` (amendment A9). The Polymarket International fallback is not built.

Every choice below is also commented next to the code that makes it (`normalize.go`, `fetcher.go`).

## Endpoints used (public, read only)
| Purpose | Request | Raw kind |
|---|---|---|
| Discovery | `GET /v1/markets?categories=macro&active=true&closed=false&limit=100&offset=N` | `markets` |
| Prices | `GET /v1/markets/{slug}/book` | `prices` |

- Scope (V1): only Federal Reserve federal funds rate markets. Polymarket US lists them in the `macro` category. Discovery pages through open macro markets (100 per page, at most 5 pages) and keeps markets whose rules mention "federal funds rate". Every other macro market is counted in the run's skipped count and summarized in one skip row per page ("N markets outside V1 scope").
- If the 5-page bound is reached, a skip row says so.
- Every 200 response body is saved byte-for-byte with `st.Raw.Save` before it is parsed. A non-200 response is not saved as market data; its status and an excerpt of its body go on the failed run or the skip.

## Requests, rate limits, failures
- Each request has its own 15 second timeout.
- Requests are paced at least 100 ms apart (10 per second), under the published 25 per second limit.
- HTTP 429: wait 1 s, 2 s, 4 s and retry; after the third retry the request fails (docs: "Rate Limits").
- A markets page that fails (network, timeout, non-200, body does not parse) fails the run: `Runs.Finish` with `failed` and the error text. It is not returned as an error; only store errors are returned. Cards from earlier pages in the same run stay saved.
- An order book that fails is one skip with the reason; the run continues. `FetchPrices` fails its run only if every requested book failed.
- Counts: `saved` = cards saved plus price records appended; `skipped` = every record not used (malformed records, out-of-scope markets, unusable book sides, failed books).

## Market card
| Field | Choice |
|---|---|
| market_id | The market **slug** (e.g. `rvcus-fedfnds-ondt-2027-12-31-gte3pt50`). Polymarket US uses the slug for the book, BBO and orders, so the price loop refreshes a card from its market_id alone |
| subject | `fed_funds_upper_bound` when the rules mention "upper bound of the target (range for the) federal funds rate". Threshold, decision-bucket and change markets all qualify; only threshold markets get a cutoff. Passed through vocab.yaml, so a missing entry stores empty |
| cutoff_op, cutoff_value, cutoff_unit | Only from exactly one sentence "federal funds rate is <op> X%". `greater than or equal to` and `at or above` -> `gte`; `greater than` and `above` -> `gt`; `less than or equal to` and `at or below` -> `lte`; `less than` and `below` -> `lt`. Value through `card.ParseDecimal` (`3.50` -> `3.5`), unit `percent` through vocab. No sentence, or more than one, leaves all three empty |
| Decision buckets | "No change", "25 bps Decrease", "50+ bps Increase", "Fed Rate Cut Greater Than 25bps", "Fed Rate Cut in 2026" have no threshold sentence, so the cutoff is empty and they never match |
| deadline | The moment the rules say the outcome is read: "<Month> <D>, <YYYY> at <h:mm> AM/PM ET" (or a comma instead of "at"). ET is America/New_York with daylight saving for that date (tzdata embedded), converted to UTC via `card.ParseInstant`. Example: "December 31, 2027 at 11:59 PM ET" -> `2028-01-01T04:59:00Z`. Exactly one distinct moment must appear, else empty. `endDate` is not used: it is a settlement window end about two weeks later. FOMC decision-bucket rules name a meeting ("currently scheduled for December 8-9") but no time, so their deadline is empty |
| decider | `federal_reserve` when the rules say "Outcome sourced from the (United States) Federal Reserve." Any other source (e.g. the Federal Reserve Bank of New York) is not mapped and stays empty |
| unclear_result | `yes_or_no_only` when the rules contain no alternative-settlement wording. Polymarket US documents that contracts settle at $1.00 or $0.00 and that any alternative settlement (50-50, last fair market price) is written in that market's own rules ("Market Rules", "Market Settlement"). Rules mentioning 50-50, $0.50, fair market price, last price, cancel, void, split or refund are left empty rather than guessed |
| title | Market `title` (e.g. "3.50% Or Above"), else `question` |
| event_title | Market `question` (e.g. "Fed Funds Rate at the End of 2027?") |
| rules_text | Market `description`, verbatim |
| category | Market `category` (`macro`) |
| volume_24h | `volume24hr` as the venue's decimal text when present (shares, display only); the October 2026 payload does not carry it, so it is empty |
| url | Empty. The API returns no page link and `polymarket.us/market/<slug>` redirects to the home page |
| raw_ref, fetched_at | The saved markets response id and its fetched_at, so `rebuild` reproduces the card |

Skipped markets: a record that does not decode (wrong JSON types), a record with no slug, and a market whose sides are not labeled Yes (long) and No (short), such as a sports market labeled by team.

## Price records
Polymarket US has **one instrument per market**. Its long side is labeled `Yes` and its short side `No` in `marketSides` (checked by label and the `long` flag, never by array position; markets without these labels are skipped). All order book prices refer to that instrument, i.e. to YES ("Orders & Trading": "the price always refers to the YES side").

| Side | buy_price |
|---|---|
| yes | Lowest offer: buying YES lifts the best offer |
| no | 1 minus the highest bid: buying NO is selling YES at the best bid |

- Prices are decimal dollar strings with currency `USD` (e.g. `{"value": "0.2700", "currency": "USD"}`), not integer-encoded. They are parsed exactly with `card.ParseMicros`; `1 - bid` is integer micro-dollar math. No float anywhere. This matches the BBO endpoint's `longQuote` and `shortQuote` (e.g. bid 0.26 -> `shortQuote` 0.74).
- Levels with zero quantity are ignored. An empty side of the book gives an empty buy_price, never 0 or 1.
- A level that does not parse, is not USD, or is outside 0 to 1 makes that side empty and is logged as a skip.
- A book whose `state` is not `MARKET_STATE_OPEN` gives empty prices on both sides: nothing is executable.
- The markets payload carries no executable ask (`marketSides[].price` and `outcomePrices` are display prices), so `NormalizeMarkets` returns no price records.
- priced_at is the book response's fetched_at.
- **Fee is empty.** The published schedule is `fee = 0.0695 x C x p x (1 - p)` rounded to the cent with banker's rounding per fill, minus volume-tier rebates that depend on the participant. The per-contract fee therefore depends on fill size and account tier and is not exactly derivable for one contract.
