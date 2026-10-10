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

## Display-only categories (fetched, never matched)
Added 2026-10-10 so the feed shows a real multi-category venue. Code: display.go and the display branch of `normalizeMarkets`.
- Config: `fetchers.polymarket.display_categories` in config/equinox.yaml, a list of `{category, limit}`. `card.FetcherConfig` is a frozen contract that holds only `source` and `base_url`, so this package parses its own section of the same file (`ParseDisplay`) and `registry.LoadDisplay` hands it to the fetcher; `equinox fetch` and `equinox watch` call it. No list means no display fetch (every test fixture and the acceptance harness). `macro` is refused: it is the Fed page.
- Categories seen on 2026-10-10 with open markets: politics, crypto, finance, culture, science, climate, geopolitics, sports, macro. Chosen: politics, crypto, finance, climate, sports, because Kalshi has the same categories.
- Request: `GET /v1/markets?categories=<c>&active=true&closed=false&limit=<n>&offset=0`, one per category, after the Fed page and its books. Default limit 10, at most 100. No other request is made for display markets.
- Ranking: the payload has no volume field (no `volume`, `volumeNum`, `volume24hr` or open interest), and the API ignores `orderBy` and `orderDirection` (the same order came back for `volumeNum` ascending, descending and a nonsense field). So the markets are the first n in the venue's own listing order, and `volume_24h` stays empty.
- Display path chosen by the saved endpoint: a markets page whose `categories` value is not `macro` is a display page. A macro page (or a raw with no endpoint) takes the Fed path exactly as before, so rebuild picks the same path as fetch and a Fed sentence on a display page never gives a subject.
- Prices: read from the markets page itself, not from order books, so a display category costs one request. Reason: on 2026-10-10 the public gateway answered HTTP 429 with `Retry-After: 10` after about 5 requests in a few seconds (measured at 1 and 2 requests per second, far below the documented 25 per second), so a book per display market would crowd out the Fed books. The page carries `bestAskQuote` and `bestBidQuote` ({value, currency}); in the recording they equal the best offer and best bid of the market's book for every market whose book was fetched (e.g. 0.8910 and 0.8900). They are read like a book: YES buy_price = `bestAskQuote`, NO buy_price = 1 minus `bestBidQuote`, parsed exactly with `card.ParseMicros`, integer math, no float. A missing quote is an empty price, never 0 or 1; a quote that is not USD, does not parse or is outside 0 to 1 empties that side and is logged as a skip; a market whose `status` is not `MARKET_STATUS_OPEN` gets empty prices. priced_at is the page's fetched_at; fee is empty. These quotes are decoded separately and only on display pages, so the Fed path still takes prices only from books. Display cards never match, so these prices are never routed.
- Failures: a display page still answered with HTTP 429 after the usual 1 s, 2 s, 4 s backoff waits 10 s (the measured `Retry-After`) and tries once more. A display page that then fails is one skip with the reason; it never marks the run failed.
- Payoff fields: every field is empty except `deadline`, so the matcher never pairs a display card. `deadline` uses the Fed rule (exactly one "<Month> <D>, <YYYY> at <h:mm> AM/PM ET" moment in the rules, converted from ET), else empty; for example "12:00 AM ET on January 1, 2027" (time before date) stays empty. It only feeds the "Closing soon" sort.
- Record fields: title, event_title, rules_text and url as for Fed markets. Markets whose sides are not labeled Yes and No are skipped as before.
- `category`, display only: `finance` is shown as `Financials` and `climate` as `Climate and Weather` (the words Kalshi uses for the same thing), so the feed's category filter groups both venues; any other value gets its first letter in upper case (`politics` to `Politics`). Fed cards keep `macro` unchanged.
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
| unclear_result | Decision D-20: filled only from a source tied to that contract. `yes_or_no_only` only when the market's own rules contain an explicit sentence of the form "If the outcome cannot be determined ..., this market resolves to Yes/No"; otherwise empty. Real Polymarket US Fed rules (2026-10-10) contain no such sentence, and no Fed-specific clause was found in the filed contract terms or the PMUS Rulebook, so every real Fed card has it empty and matches nothing. The venue's general docs ($1.00 or $0.00) are not treated as contract specific |
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
- On the Fed (macro) page the markets payload is not used for prices (`marketSides[].price` and `outcomePrices` are display prices), so `NormalizeMarkets` returns no price records there. Display-only pages are the exception (see "Display-only categories").
- priced_at is the book response's fetched_at.
- **Fee is empty.** The published schedule is `fee = 0.0695 x C x p x (1 - p)` rounded to the cent with banker's rounding per fill, minus volume-tier rebates that depend on the participant. The per-contract fee therefore depends on fill size and account tier and is not exactly derivable for one contract.
