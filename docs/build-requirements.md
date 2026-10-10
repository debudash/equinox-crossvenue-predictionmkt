# Equinox - Build Requirements (Final)

Oct 9, 2026 · @Debu Dash

> This document is authoritative. If any other file in this repo conflicts with it, this document wins.

## Goal and governing principle

Show that Polymarket and Kalshi markets can be compared through one shared market card, that some markets can be recognized as the same bet, and that a routing choice between them can be simulated from saved prices. The system is a simulation. It does not place orders.

**Governing principle: precision over recall.** A false match routes a user into a different bet. A missed match only loses an opportunity. When the system is not sure, it does not route, and it records why.

Build these requirements as written. Do not replace the match rule with a similarity score, a language model, or a percentage.

## Locked choices

1. The venues are Polymarket and Kalshi.
2. A market is stored as one card. The rules decide the bet. The title is only a label.
3. Two markets are the same bet only when their payoff fields are equal. They are opposite bets only when their payoff fields are equal except that one cutoff is the exact complement of the other.
4. There is no percentage match. There is no language model in the match or in the route.
5. The router runs only on cards and prices that are already saved. It does not call a venue while choosing.
6. If the system is not sure, it does not route. It records why.
7. Only the venue fetchers know how each venue formats its data. The matcher and the router do not.
8. Both fetchers normalize into one shared vocabulary. A value outside the vocabulary is stored as empty.
9. Every raw venue response is saved, so cards can be rebuilt without calling a venue.

## System shape

Four separate pieces, run in order. Each reads only what the previous pieces saved.

1. **Fetchers.** One for Polymarket, one for Kalshi. Each downloads open markets and prices from the venue's public API, saves the raw response, and writes cards and price records to local storage.
2. **Matcher.** Reads cards only. Writes a match result for each pair it compares.
3. **Router.** Reads match results and price records only. Writes a routing decision and a reason.
4. **Feed.** Reads cards, match results and decisions only. Displays them. Writes nothing.

A failed download in one fetcher must not stop the other fetcher, the matcher, the router or the feed.

**Stack.** Go, the sponsor's stack. SQLite for local storage. The feed is server-rendered HTML from Go templates, with no frontend framework.

**Commands:**

| Command | Does |
| --- | --- |
| `equinox fetch` | Runs both fetchers |
| `equinox match` | Compares saved cards |
| `equinox route --side yes --as-of <time>` | Routes every `same` or `opposite` pair for the given side; `--as-of` defaults to the current time |
| `equinox serve` | Starts the read-only feed |
| `equinox rebuild` | Rebuilds cards from saved raw responses, without calling a venue |

## Market card and price record

Every market from either venue is stored as one card. If a fetcher cannot find or normalize a field, it stores the field as empty. It never invents a value.

**Payoff fields** (decide the match):

| Field | Meaning | Format |
| --- | --- | --- |
| subject | What is measured | An entry from the shared vocabulary, e.g. `fed_funds_upper_bound` |
| cutoff\_op | Comparison | `gt`, `gte`, `lt` or `lte` |
| cutoff\_value | Threshold | Decimal string with no trailing zeros, e.g. `3.5` |
| cutoff\_unit | Unit of the threshold | `percent`, `usd`, `count` (from the vocabulary) |
| deadline | The moment the outcome is read | ISO 8601, converted to UTC |
| decider | Who or what source settles the market | An entry from the shared vocabulary, e.g. `federal_reserve` |
| unclear\_result | What is paid if the result is unclear, cancelled or split | `yes_or_no_only`, `half`, `last_price` or `cancelled` |

**Record fields** (never used to match):

| Field | Meaning |
| --- | --- |
| venue | `polymarket` or `kalshi` |
| market\_id | The venue's own id for this market |
| title | Display name only |
| rules\_text | The full settlement text, verbatim |
| raw\_ref | Pointer to the saved raw response this card came from |
| fetched\_at | When the card was saved |
| event\_title, category, volume\_24h, url | Display only, for the feed |

Empty payoff fields are allowed on a card. They are never allowed to match.

**Price record.** One record per market per side.

| Field | Meaning |
| --- | --- |
| venue, market\_id | Same as the card |
| side | `yes` or `no` |
| buy\_price | Cost to buy that side, in dollars from 0 to 1. Empty if unknown |
| fee | Known fee per contract, in dollars. Empty if unknown |
| priced\_at | When this price was saved |

**Shared vocabulary.** One venue-agnostic file, `vocab.yaml`, lists every allowed `subject`, `decider` and `cutoff_unit`. Both fetchers map into it. V1 seeds it with one category, Federal Reserve rate markets. Coverage grows only by adding vocabulary entries plus fetcher rules, each with a fixture test.

**Fetcher documentation.** Each fetcher documents every normalization choice next to its code. Examples: which venue time is the deadline, how a rules sentence maps to `unclear_result`, and which records are skipped (Kalshi multivariate combo markets are skipped and counted).

## Equivalence

The matcher compares a Polymarket card with a Kalshi card. The result is exactly one of `same`, `opposite` or `not_same`.

**Which pairs are compared.** Only pairs with the same non-empty `subject`. Pairs with different subjects are not compared and not logged, but the count of cards per subject is logged on every run.

### `same`

All payoff fields are non-empty and equal on both cards: subject, cutoff\_op, cutoff\_value, cutoff\_unit, deadline, decider, unclear\_result. YES on one card pays exactly when YES on the other pays. Titles may differ.

### `opposite`

All payoff fields are non-empty and equal except the cutoff operator, and the two operators are exact complements at the same value and unit:

| Card A | Card B |
| --- | --- |
| `gt` | `lte` |
| `gte` | `lt` |

YES on one card pays exactly when NO on the other pays. The router therefore compares YES on one venue with NO on the other.

### `not_same`

Every other compared pair. This includes:

- Any payoff field empty on either card
- Same title with a different subject, cutoff, deadline, decider or unclear-result rule
- One cutoff stricter than the other, e.g. `gt 3.5` against `gte 3.5`
- One deadline earlier than the other
- The same event settled by different deciders

`not_same` is a finished answer. The matcher never attaches a confidence score.

### Comparison rules

- Compare normalized field values only. Never compare titles or raw rules text.
- The matcher never reads `rules_text` to overrule a field. If a field is wrong, fix the fetcher.
- The same two cards always produce the same result.
- Order does not matter. Comparing A with B gives the same result as comparing B with A.

## Routing

The router answers one question: to buy this position, which venue is cheaper, and why.

**Inputs:**

- A match result for a pair of cards
- The requested side, `yes` or `no`, stated relative to the Kalshi card
- The latest saved price record for each needed card and side
- `as_of`, the reference time, passed in (the router never reads the clock)

**Price age limit: 10 minutes.** A price with `priced_at` more than 10 minutes before `as_of` is treated as empty. A fetch, match and route session completes within minutes, so an older price is not a current price.

### Decision rules, applied in order

1. If the match result is `not_same`, do not route. Reason: the bets are not the same.
2. Pick the two prices to compare:
   - `same`: the requested side on Kalshi and the same side on Polymarket.
   - `opposite`: the requested side on Kalshi and the other side on Polymarket. They are the same position.
3. If either buy price is empty (unknown or older than the age limit), do not route. Reason: a price is missing or stale, naming which.
4. Cost is `buy_price` plus `fee` when fees are known on both venues. If either fee is empty, compare `buy_price` only on both venues, and the log says fees were not applied. This keeps the comparison like for like.
5. Choose the venue with the lower cost.
6. If the costs are equal, choose Kalshi. This tie break is fixed so the choice never flips between runs.
7. Write one decision record.

**The router never:**

- Calls Polymarket or Kalshi
- Reads the clock
- Reads a title and decides markets might be close enough
- Changes a `not_same` result
- Uses a midpoint or last trade when `buy_price` is empty

The router prices one contract at the saved buy price. It does not model order size or book depth.

## Decision log

Every compared pair and every routing decision writes one record. A refusal is recorded the same way as a choice.

| Field | Meaning |
| --- | --- |
| decided\_at | When the record was written |
| as\_of | Reference time used by the router (empty for match records) |
| market\_a | `kalshi` and market id |
| market\_b | `polymarket` and market id |
| match\_result | `same`, `opposite` or `not_same` |
| side\_a, side\_b | Sides compared on each venue (empty for match records) |
| price\_a, price\_b | Buy prices used, or empty |
| priced\_at\_a, priced\_at\_b | Times of the prices used, or empty |
| fee\_applied | `yes` or `no` |
| action | `buy_on_kalshi`, `buy_on_polymarket` or `no_route` |
| reason | One or two plain sentences. For `not_same`, name the field that differed or was empty. For `no_route`, name the missing or stale price |

The log is append-only. Records are never edited.

## Failure behavior

Bad data never stops a run and never reaches a decision.

- If one venue's API fails, save the error, keep the other venue's data, and continue.
- Do not route a pair when either card or either price is missing because of that failure.
- If saved data is inconsistent (unknown venue, side other than yes or no, price outside 0 to 1, deadline that does not parse), skip that record, log the problem, and continue.
- Every fetcher run logs counts of records saved and skipped, with a reason for each skip. No record disappears silently.
- One bad market never stops the matcher or the router.
- The matcher and the router never retry a venue call, because they never make one.

## Feed

A read-only, Reddit-style page shows every saved market from both venues, with match and routing results in place. Polish is not a goal.

| Reddit element | Equinox meaning |
| --- | --- |
| Subreddit | `category` (display field) |
| Post | One venue event (`event_title`), with a venue flair |
| Comments | The event's markets, each with saved yes and no buy prices and `priced_at` |
| Crosspost badge | The market has a `same` or `opposite` match on the other venue |
| Score | `volume_24h` |
| Bot reply under a market | The latest match and routing records for it, showing `reason` and `action` verbatim from the log |

Sorts: Top (by `volume_24h`), Closing soon (by `deadline`), Matched (posts with a crosspost badge first). Filters: venue and category.

The feed only reads. It never computes a match or a route, so everything it shows traces to a saved record.

## Tests the build must include

All tests use saved fixtures and a fixed `as_of`. No test calls a live venue.

**Matcher**

1. Equal payoff fields match as `same`, even when the titles differ.
2. `gt 3.5` on one card and `lte 3.5` on the other, all other payoff fields equal, match as `opposite`.
3. Same title with different deciders is `not_same`, and the reason names `decider`.
4. Same title with different deadlines is `not_same`, and the reason names `deadline`.
5. `gt 3.5` against `gte 3.5` is `not_same`.
6. An empty deadline is `not_same`, and the reason names `deadline` as empty.
7. Comparing A with B and B with A gives the same result.

**Router**

8. `not_same` produces `no_route`.
9. A missing buy price produces `no_route`.
10. A price older than 10 minutes before `as_of` produces `no_route`, with the reason "price is stale."
11. The lower cost wins. Known fees on both venues can change which venue wins.
12. If a fee is known on only one venue, both are compared on `buy_price` only, and `fee_applied` is `no`.
13. For an `opposite` pair, buying yes compares Kalshi yes with Polymarket no.
14. Equal costs choose Kalshi.
15. Running the router twice on the same saved inputs and `as_of` returns the same action and reason.

**Fetchers and failure**

16. The Kalshi and Polymarket fixtures for the same Fed market normalize to byte-equal payoff fields.
17. A value not in `vocab.yaml` is stored as empty, and that card matches nothing.
18. `equinox rebuild` on saved raw responses reproduces identical cards.
19. One venue failing to load does not drop the other venue's saved cards, and pairs that needed the failed venue are `no_route`.
20. A malformed record is skipped, counted and logged, and the run completes.

## Assumptions and done-when

**Assumptions to write in the code:**

- The payoff fields, not the title, are the bet.
- "The same bet" means the same payoff, not a related event.
- A fee changes the cost. It never changes whether two markets match.
- Kalshi wins an exact cost tie.
- Empty means unknown. Unknown never counts as equal.
- A price older than 10 minutes is unknown.
- Both venues pay 1 dollar per winning contract (USD on Kalshi, USDC on Polymarket), treated as equal.
- This build simulates a choice. It does not claim the choice can be executed live.

**Done when:**

- A person runs `equinox fetch`, then `equinox match`, then `equinox route`, and the route makes no venue call.
- The log shows, in plain sentences, which pairs matched, which were refused and why, and which venue was chosen for each matched pair.
- `equinox serve` shows both venues' markets, with crosspost badges and log reasons on matched markets.
- All 20 tests pass.
- The README states the vocabulary coverage, every fetcher normalization choice, and the known limitations listed under Out of scope.

## Changes from the locked spec

This document keeps the locked spec's rules and adds only fixes for gaps that would break its own principles. Each change is listed for review.

| Change | Why |
| --- | --- |
| **`opposite` redefined** as complementary cutoffs (`gt`/`lte`, `gte`/`lt`) with all other fields equal | The locked text defined `opposite` as two cards with identical fields, one yes and one no. Identical propositions on opposite sides are opposite positions, not the same position. Routing on that rule would buy the wrong bet. |
| **`side` moved from the card to the price record** | A card is a proposition. Each side has its own price. This makes the `opposite` rule unambiguous. |
| **Requested side added as a router input** | Without it, the router cannot know which position it is pricing. |
| **Structured cutoff** (`cutoff_op`, `cutoff_value`, `cutoff_unit`) | Free-text cutoffs from two fetchers rarely match as strings, so true matches would be lost. |
| **Shared vocabulary for `subject`, `decider`, `cutoff_unit`** | Same reason. Equality is only meaningful if both fetchers use the same words. |
| **Price age limit of 10 minutes, with `as_of` passed in** | The locked spec routed on the latest saved price of any age. Passing `as_of` keeps the router deterministic. |
| **Fees applied only when known on both venues** | Comparing a fee-inclusive cost with a fee-free one is not like for like. |
| **Raw responses saved, plus `equinox rebuild`** | "Fix the fetcher" needs the original data. Settled markets may no longer be returned by the venue. |
| **Pairs compared only within a subject** | Keeps the log readable. Comparing every Kalshi card with every Polymarket card would log mostly noise. |
| **Read-only feed added** | Requested display. It never computes matches or routes. |
| **Tests expanded from 12 to 20** | One test per change above, plus symmetry and malformed-record tests. |

## Out of scope

State each of these as a known limitation in the README.

- Sending orders, wallets, accounts or moving money
- A third venue
- A similarity percentage, embedding distance or language-model verdict
- Treating "close" markets as the same bet (tighter cutoff, earlier deadline, different data source)
- One-to-many matches (one market against a basket of markets on the other venue)
- Order size, book depth, and maker versus taker fills
- Spread or arbitrage statistics
- Kalshi multivariate combo markets
- Choosing a route from live calls made during the decision
- Production UI, regulatory or compliance claims

## Amendments (approved Oct 9, 2026)

These add to the requirements above. They do not change the match rule, the routing rules, the 20 required tests, or the done-when criteria.

| Amendment | What it adds | Why |
| --- | --- | --- |
| **A1. `equinox watch`** | A background scheduler with two loops. Discovery (default every 30 minutes): fetch market lists, save raw responses and cards, then run match only if cards were added or changed. Prices (default every 2 minutes): refresh prices for markets in a `same` or `opposite` pair, save a price record on every poll even when unchanged, and re-run route for a pair when a needed price changed, with `as_of` set to the refresh time. | Keeps the feed current without a person running commands, and keeps matched prices inside the 10-minute age limit. |
| **A2. Cycles** | Every fetch, match and route pass, whether from the CLI or from `watch`, completes as a numbered cycle recorded in storage. A cycle is visible to readers only when it has fully completed. | Gives the feed a consistent point in time to read, so a bot reply never disagrees with the prices shown beside it. |
| **A3. In-memory feed cache** | `equinox serve` holds a read-only in-memory snapshot of cards, latest prices and latest log records, built from SQLite for the latest completed cycle. It checks for a newer completed cycle every few seconds and swaps the snapshot atomically. Scrolling and "Load more" page through the snapshot only. | Fast scrolling. The cache never holds data that is not in SQLite, and it never calls a venue. |
| **A4. New-data banner** | When a newer cycle completes after a page was rendered, the page shows "New data saved at [time]. Reload." | The reader decides when the view changes; nothing shifts mid-scroll. |
| **A6. Watch routes both sides** | On each price change for a matched pair, `watch` runs route for `yes` and for `no`, writing two route records. | No person is present to choose a side, and the feed's bot replies should cover both positions. |
| **A7. Separate processes** | `equinox watch` and `equinox serve` run as separate processes. SQLite is the only handoff between them. | A scheduler crash never takes the feed down; the feed keeps serving the last completed cycle. |
| **A8. Default intervals** | Discovery every 30 minutes, prices every 2 minutes, retention 24 hours. `watch` refuses to start if the price interval is not below the 10-minute price age limit. | The price interval must keep matched prices fresh. |
| **A9. Polymarket US** | The Polymarket fetcher reads Polymarket US, the CFTC-regulated exchange, through its public market data API (no API key). The `venue` value stays `polymarket`. Polymarket US contracts settle in USD, so the payout assumption becomes 1 USD per winning contract on both venues. If Wave 1 finds no Fed market on Polymarket US that fits the cutoff format, the fetcher falls back to the Polymarket International public APIs, and the README states which venue was used. | Polymarket US is the venue a US platform such as Apex could route to, and it settles centrally like Kalshi rather than through an oracle vote. |
| **A5. Retention** | Price records and price raw responses older than 24 hours are pruned by `watch`. Card raw responses are kept so `equinox rebuild` still works. Log records are never pruned. | Bounded storage when `watch` runs for days. |

**Unchanged by the amendments:** the feed writes nothing; the matcher and router never call a venue or read the clock; `watch` is the only background writer, so SQLite has one writer at a time; the CLI commands still work on their own and take the same run lock as `watch`.

**Additional tests for the amendments** (named `TestAmd_`):

- W1. Match runs after a discovery cycle that changed cards, and does not run after one that changed nothing.
- W2. An unchanged price is still saved on each poll, so a matched pair does not go stale while its price is steady.
- W3. A changed price re-runs route for that pair only, with `as_of` equal to the refresh time.
- W4. A venue failure in one cycle is saved and counted, and the next cycle runs normally.
- W5. The feed cache only ever shows a fully completed cycle; a cycle that fails midway is never shown.
- W6. A "Load more" request that carries an older cycle number pages through that cycle's snapshot, so scrolling never skips or repeats a post.
- W7. A CLI run while `watch` holds the run lock is refused with a reason, and nothing is written.
- W8. Pruning removes only price records and price raw responses older than 24 hours.
- W9. A price change on a matched pair produces two route records, one for `yes` and one for `no`, with the same `as_of`.
- W10. With `watch` stopped or crashed, `serve` keeps serving the last completed cycle and shows when the last cycle completed.
- W11. `watch` refuses to start when the price interval is not below the price age limit, naming both values.
