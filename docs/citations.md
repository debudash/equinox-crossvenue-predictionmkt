# Citations

This file has two parts. Business sources are cited inline as bracketed tags and mapped to the ADRs and docs that use them. Technology references are kept as background reading only and are not cited in any ADR. ADR-002, ADR-005 and docs/architecture.md carry no citations; ADR-004 and ADR-006 each cite one regulatory source as a business analogy. The "Used in" column is generated from the docs.

## Business sources (cited inline)

Every bracketed tag used in the docs resolves here. Status:
- Checked: the source was opened during research on 2026-10-07 and the cited claim was seen in it.
- Verify: cited from general knowledge; confirm the exact reference before submission.
- Secondary: seen in a third-party source; prefer the official source before submission.

| Tag | Reference | URL | Status | Used in |
|---|---|---|---|---|
| Fellegi-Sunter, record linkage | Fellegi and Sunter (1969), "A Theory for Record Linkage", JASA. Summarized with blocking in US Census Bureau research reports | https://www.census.gov/content/dam/Census/library/working-papers/1993/adrm/rr93-8.pdf | Checked (via Census summary) | ADR-001, ADR-007, spec |
| Orrery, exact/near/related | Orrery multi-venue matching policy: exact matches comparable, near and related labeled | https://orrery.me/venues | Checked | ADR-001, spec |
| Semantic Non-Fungibility, arXiv 2601.01706 | "Semantic Non-Fungibility and Violations of the Law of One Price": human-validated cross-platform aligned market dataset; equivalence and subset relations | https://arxiv.org/pdf/2601.01706 | Checked | ADR-001, ADR-007, spec |
| Event finality, arXiv 2610.00183 | "Two Models of Event Finality": Polymarket oracle path vs Kalshi native finality | https://arxiv.org/html/2610.00183 | Checked | spec |
| Underspecified rules, arXiv 2607.06166 | "When do prophets profit in prediction markets?": excluded underspecified resolution rules; expects conventions to standardize | https://arxiv.org/pdf/2607.06166 | Checked | spec |
| Resolution trap, Fed cut example | NexusFi Academy, cross-platform arbitrage: differing resolution criteria for the same Fed decision | https://nexusfi.com/a/prediction-markets/arbitrage-prediction-markets | Checked (practitioner source) | ADR-001, spec |
| Predictions.io, cross-venue gaps | Decrypt / Chainwire on Predictions.io: equivalent questions priced apart; fee calculator incl. Kalshi taker formula | https://decrypt.co/376808/same-election-question-two-different-odds-predictions-io-launches-free-cross-venue-comparison-tools | Checked (press release); verify fees against venue docs | ADR-001, ADR-003, spec |
| lslb05, one-to-one matching | prediction-markets-arb repo: human promotion gate, one-to-one constraint, outcome by label not position, yes_bid 0 is empty | https://github.com/lslb05/prediction-markets-arb | Checked | Not cited in current docs (kept for reference) |
| Polymarket, UMA resolution | Polymarket API architecture: Gamma, CLOB, Data APIs; UMA Optimistic Oracle resolution | https://medium.com/@gwrx2005/the-polymarket-api-architecture-endpoints-and-use-cases-f1d88fa6c1bf | Secondary; confirm at docs.polymarket.com | spec |
| MiFID II Art.27, best execution | MiFID II Article 27(1) execution factors | https://handbook.fca.org.uk/handbook/COBS/11/2A.html | Checked (as transposed in FCA COBS) | ADR-003, spec |
| FCA COBS 11.2A, total consideration | FCA Handbook COBS 11.2A: total consideration for retail orders | https://handbook.fca.org.uk/handbook/COBS/11/2A.html | Checked | ADR-003, spec |
| FINRA 5310, best execution | FINRA Rule 5310 Best Execution and Interpositioning | https://www.finra.org/rules-guidance/rulebooks/finra-rules/5310 | Checked 2026-10-10: 5310(a)(1) "use reasonable diligence to ascertain the best market"; factors include character of the market (price, volatility, liquidity) and size and type of transaction | ADR-003 |
| FINOS CDM, open derivatives model | FINOS Common Domain Model; ISDA milestone update incl. exchange-traded derivatives | https://www.isda.org/?p=1076175 | Checked | ADR-007, spec |
| CFTC NPRM, June 2026 | CFTC, "Prediction Markets; Public Interest Determinations", Federal Register 2026-11854 | https://www.federalregister.gov/documents/2026/06/12/2026-11854/prediction-markets-public-interest-determinations | Checked | ADR-007, spec |
| Dome acquisition, venue absorbs aggregator | Dome docs: acquired by Polymarket, APIs end of life April 28, 2026 | https://docs.domeapi.io/index.md | Checked | ADR-007, spec |
| Tradeweb-Kalshi, distribution | CasinoBeats on Dome deal and same-day Tradeweb-Kalshi data integration | https://casinobeats.com/2026/02/20/polymarket-acquires-dome-to-expand-developer-tools-for-prediction-markets/ | Checked (trade press) | ADR-007, spec |
| pmxt, unified API | pmxt: open-source unified API for prediction markets | https://github.com/pmxt-dev/pmxt | Checked | ADR-007, spec |
| IBKR, multi-venue access | Interactive Brokers prediction markets: Kalshi, CME Group, ForecastEx in one account | https://www.interactivebrokers.com/predictionmarkets/en/home.php | Checked | ADR-007, spec |
| Reg NMS 611, self-help | SEC Regulation NMS Rule 611 order protection and self-help exception | https://www.ecfr.gov/current/title-17/section-242.611 (read via https://www.law.cornell.edu/cfr/text/17/242.611) | Checked 2026-10-10: 611(b)(1) excepts a trade-through while the other trading center "was experiencing a failure, material delay, or malfunction of its systems or equipment". The rule text does not use the words "self-help"; that is the industry name | ADR-004 |
| SR 26-2, model risk | Federal Reserve SR 26-2, Revised Guidance on Model Risk Management (2026-04-17); supersedes and replaces SR 11-7 (2011) | https://www.federalreserve.gov/supervisionreg/srletters/SR2602.htm | Checked 2026-10-10: the letter says it "supersedes and replaces SR letter 11-7" and takes a risk-based approach to model risk management. The detailed expectations in its attachment were not re-read. The old sr1107.htm link returns 404 | ADR-006 |
| DeFi Rate, settlement divergence | DeFi Rate, "How Event Contracts and Dispute Resolution Work at Prediction Markets" (updated Sep 28, 2026): Cardi B halftime market settled at last traded price on Kalshi and YES at $1 on Polymarket; centralized vs oracle settlement compared | https://defirate.com/prediction-markets/how-contracts-settle/ | Checked | ADR-001, spec |
| Polymarket US, CFTC venue | Polymarket US: operated by QCX LLC d/b/a Polymarket US, a CFTC-regulated Designated Contract Market; Fed and economics markets listed | https://polymarket.us/category/economics | Checked | spec |

## Technology references (background, not cited in ADRs)

| Reference | URL | Status | Relevant to |
|---|---|---|---|
| Kalshi API market model: response_price_units, floor_strike, cap_strike, rules_primary | https://docs.kalshi.com/python-sdk/models/Market | Checked | Kalshi fetcher |
| Polymarket Gamma metadata vs CLOB book; empty side reported as 0 or 1; metadata can lag the book | https://apify.com/neverempty/polymarket-weather-markets | Secondary; confirm at https://docs.polymarket.com | Fallback fetcher (Polymarket International) only |
| Dune prediction_markets cross-venue schema with venue-scoped ids | https://docs.dune.com/data-catalog/curated/prediction-markets/prediction_markets/overview.md | Checked | Card ids and schema |
| Martin Fowler, "Event Sourcing" | https://martinfowler.com/eaaDev/EventSourcing.html | Background reference, not a claim; URL resolves (checked 2026-10-10) | Raw responses and rebuild |
| FIX Protocol data types (Price as decimal) | https://www.fixtrading.org/standards/ | Background reference, not a claim; URL resolves (checked 2026-10-10) | Decimal prices |
| Michael Nygard, "Release It!" (circuit breaker pattern) | https://pragprog.com/titles/mnee2/release-it-second-edition/ | Background reference, not a claim; URL resolves (checked 2026-10-10) | Failure isolation |
| OWASP Top 10 for LLM Applications, LLM01 Prompt Injection | https://genai.owasp.org/llm-top-10/ | Background reference, not a claim; URL resolves (checked 2026-10-10) | Background for ADR-006 (no model in decisions) |
| NIST AI Risk Management Framework 1.0 | https://www.nist.gov/itl/ai-risk-management-framework | Background reference, not a claim; URL resolves (checked 2026-10-10) | Background for ADR-006 (no model in decisions) |
| Simon Brown, The C4 model | https://c4model.com | Background reference, not a claim; URL resolves (checked 2026-10-10) | Architecture diagrams |
| Polymarket US API: public market data API (no key) for markets, events, order books, BBO, settlement and price history. Prices observed 2026-10-10 are decimal dollar strings with currency USD, not integer encoded | https://docs.polymarket.us/api-reference/introduction | Checked | Polymarket US fetcher |
| Kalshi FED contract terms, Rule 100.5 (KXFED): "If no data is available on the Expiration Date at the Expiration Time, the Contract resolves to No." | https://assets.kalshi.com/contract_terms/FED.pdf | Checked 2026-10-10 | D-20, Kalshi unclear_result |
| Kalshi ECONSTAT contract terms (KXFEDFUNDSYEAR), Contingencies: "If an Expiration Value cannot be determined on the Expiration Date, Kalshi has the right to determine payouts pursuant to Rule 6.3(b)" | https://assets.kalshi.com/contract_terms/ECONSTAT.pdf | Checked 2026-10-10 | D-20, Kalshi unclear_result |
| QCX LLC d/b/a Polymarket US contract terms filed with the CFTC; terms defer to the PMUS Rulebook; no Fed-specific fallback found | https://www.cftc.gov/filings/ptc/ptc0706269006.pdf | Checked 2026-10-10 | D-20, Polymarket US unclear_result |
| Cross-venue practice (vendor commentary): matching by event description plus settlement-date window, graded exact / near / related; resolution-criteria differences named as the main risk. No venue or aggregator publishes its matching method | https://simplefunctions.dev/technicals/cross-venue-edge-detection-kalshi-polymarket | Secondary | D-20 rationale, README |
