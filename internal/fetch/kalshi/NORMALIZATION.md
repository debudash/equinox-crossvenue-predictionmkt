# Kalshi normalization choices

Every choice below is also commented next to the code that makes it (normalize.go, kalshi.go). Confirmed against the live public API on 2026-10-10.

## Source and scope
- Base URL: `https://api.elections.kalshi.com/trade-api/v2` (public market data, no key). `fetchers.kalshi.base_url` in config overrides it; the entry may be absent.
- Scope (V1): Federal Reserve rate markets only, from two series:
  - `KXFED`: "Will the upper bound of the federal funds rate be above X% following the Fed's <date> meeting?"
  - `KXFEDFUNDSYEAR`: "Will the upper bound of the target range for the federal funds rate in effect at 11:59 PM ET on December 31, <year> be above X%?" Included because it is the only Kalshi Fed threshold series whose reading moment (year end) lines up with the Polymarket US Fed threshold markets.
- Request: `GET /events?series_ticker=<series>&status=open&with_nested_markets=true&limit=200`, one series after the other. One call returns the event title, category, settlement sources and the markets.
- Pagination: follows `cursor`, at most 5 pages per series (each Fed series has about 20 open events, so one page today). If a cursor is still left after 5 pages, one skip is recorded saying the rest was not fetched.
- Price refresh (watch price loop): `GET /markets?tickers=<a,b,...>`, up to 100 tickers per request, saved as kind `prices`. A requested market the venue does not return is recorded as a skip.
- Each request has its own timeout (20 s default, `Fetcher.Timeout`).

## Display-only categories (fetched, never matched)
Added 2026-10-10 so the feed shows a real multi-category venue. Code: display.go and the display branch of `normalizeMarket`.
- Config: `fetchers.kalshi.display_categories` in config/equinox.yaml, a list of `{name, series, max_events, events_per_series}`. `card.FetcherConfig` is a frozen contract that holds only `source` and `base_url`, so this package parses its own section of the same file (`ParseDisplay`) and `registry.LoadDisplay` hands it to the fetcher; `equinox fetch` and `equinox watch` call it. No list means no display fetch (every test fixture and the acceptance harness). A Fed series in the list is refused.
- Choice of series: Kalshi has no category filter on `/events`, so the config lists series per category. They were ranked by lifetime series volume (`GET /series?category=<c>&include_volume=true`) and kept only if they had open events on 2026-10-10. Fed series (and KXFEDDECISION) are left out. Series close over time; a series with no open events simply saves nothing.
- Request: `GET /events?series_ticker=<s>&status=open&with_nested_markets=true&limit=<n>`, one per series, in config order, where n = min(events_per_series, events left in the category). Kalshi lists open events soonest first in practice, so these are the next events to settle. `limit` is in the saved endpoint, so the raw response holds exactly what was normalized and rebuild reproduces it without config. Requests stop when the category has `max_events` events (defaults: 10, and events_per_series = max_events).
- A series that still has a cursor gets one skip: "only the first n open events fetched (display bound)".
- Pacing: display requests are 200 ms apart (`Fetcher.DisplayGap`), and HTTP 429 is retried after 1 s, 2 s, 4 s. The two Fed requests are unchanged (no pacing, no retry).
- Failures: a display request that fails (network, timeout, non-200, body does not parse) is one skip with the reason and the next series is fetched. It never marks the run failed, so a display hiccup cannot fail the Fed fetch.
- Payoff fields: every field is empty except `deadline`. Subject, cutoff, unit, decider and unclear_result have no rule for these series, and the decider is left empty even when a settlement source would map, so the matcher (which needs a subject) never pairs a display card. `deadline` is the market `close_time` (when trading stops), for the feed's "Closing soon" sort only; it is not checked against the rules text like the Fed series. A close_time that does not parse leaves it empty.
- Record fields and prices: the same rules as the Fed series (title, rules_text, event_title, `category` as the event's own string such as `Climate and Weather`, `volume_24h_fp`, series url; yes and no asks from the nested market, 0 and 1 stored empty, fee empty). Multivariate and non-binary markets are skipped as before.

## Raw responses
- Every 200 response is saved byte for byte with `st.Raw.Save` before it is parsed, including one that then fails to parse.
- A non-200 response is not saved as a raw response (it holds no market data and would only add noise to rebuild); its status and the first 200 bytes of its body go in the run's error text.
- `raw_ref` is the raw response id in decimal. `fetched_at` and every `priced_at` are the time of the raw response, so rebuild reproduces them exactly.

## Payoff fields
| Field | Rule |
|---|---|
| subject | `fed_funds_upper_bound` for the two series above, set only when `rules_primary` contains "upper bound of the target". Any other series (display-only): empty, like every payoff field but the display deadline. Then filtered through vocab.yaml. |
| cutoff_op | From `strike_type`: `greater` to `gt`, `greater_or_equal` to `gte`, `less` to `lt`, `less_or_equal` to `lte`. Any other strike type (`between`, `structured`, `custom`, ...): empty. Every Fed market seen live is `greater` ("above X%"), so every Kalshi Fed card is `gt`. |
| cutoff_value | `floor_strike` for greater types, `cap_strike` for less types, read as a JSON number token (never float64) and passed through `card.ParseDecimal`, e.g. `3.75`, `4`. A missing strike or one with an exponent stays empty. |
| cutoff_unit | `percent` for both series (the rules state the threshold in percent), filtered through vocab.yaml. |
| deadline | `KXFED`: the event `strike_date`, the FOMC statement release (2:00 PM ET, e.g. `2026-12-09T19:00:00Z`). The market `close_time` is 5 minutes earlier and `expected_expiration_time` 5 minutes later, so neither is the moment the outcome is read. `KXFEDFUNDSYEAR`: the market `close_time`, which equals the rules' "11:59 PM ET on December 31" (e.g. `2028-01-01T04:59:00Z`); these events have no `strike_date`. Converted to UTC with `card.ParseInstant`. An empty value stays empty; a value that does not parse skips the record. |
| decider | From the event `settlement_sources`: "Federal Reserve Board of Governors" maps to `federal_reserve`. Any other name, or sources that map to different deciders, leave it empty. Then filtered through vocab.yaml. |
| unclear_result | Set per series from the filed contract terms named by the series `contract_terms_url`, because the API rules text (`rules_primary`, `rules_secondary`) never covers it. KXFED: `yes_or_no_only`, from FED.pdf Rule 100.5, Payout Criterion: "If no data is available on the Expiration Date at the Expiration Time, the Contract resolves to No." KXFEDFUNDSYEAR: empty, because ECONSTAT.pdf (Contingencies) says "If an Expiration Value cannot be determined on the Expiration Date, Kalshi has the right to determine payouts pursuant to Rule 6.3(b)", which is discretionary. Terms checked 2026-10-10. Limitation: `yes_or_no_only` does not record which way an unclear result falls (KXFED falls to No). |

## Record fields
- `title`: market `title`. `rules_text`: `rules_primary`, a blank line, then `rules_secondary`, both verbatim.
- `event_title`: event `title`. `category`: event `category` (e.g. `Economics`). `volume_24h`: `volume_24h_fp` as the venue wrote it.
- `url`: `https://kalshi.com/markets/<series ticker in lower case>`, the series page. Display only.

## Prices
- Read from the same events response (the nested markets carry the top of book), so a discovery fetch writes cards and prices together. No order book call is needed.
- `buy_price` yes = `yes_ask_dollars`; no = `no_ask_dollars`. Both are dollar decimal strings, parsed with `card.ParseMicros`, never float. Kalshi derives the no ask from the best yes bid, so it is executable against resting yes bids.
- An empty ask side is stored as empty. Kalshi reports a side with no offers as `1.0000` (and a side with no bids as `0.0000`, which makes the other side's ask `1.0000`), so an ask of exactly 0 or 1 is stored as empty, never as a price.
- An ask that does not parse or lies outside 0 to 1 skips the whole market (card and prices) as inconsistent.
- Fees: empty. The market payload carries no fee. The series reports `fee_type: quadratic_with_maker_fees` and `fee_multiplier`, but the coefficient and the rounding (up to the next cent per order, not per contract) come from Kalshi's published fee schedule, not from the API, so a per-contract fee cannot be derived exactly from saved data. The router then compares buy prices only.

## Skipped records (each one counted and saved in `skips` with a reason)
- Multivariate combo markets: any market with `mve_collection_ticker` or a non-empty `mve_selected_legs`.
- `market_type` other than `binary`.
- A market with no `ticker`.
- A market or event whose JSON does not decode (one bad market drops only itself; one bad event drops only its markets).
- An ask that does not parse or is outside 0 to 1.
- A deadline that does not parse.
- Pagination cut off after 5 pages (one skip for the series).

## Failures
- Network error, timeout (`ErrTimeout`), non-200 status (`HTTPError`) or a response that does not parse at all: recorded on the run as `failed` with the error text, not returned. The other series is still fetched. Cards and prices already saved in the run stay.
- Only store errors are returned from `Fetch` and `FetchPrices`.
