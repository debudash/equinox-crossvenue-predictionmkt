# Event Contract Equivalence Specification - RFC draft v0.2

Status: Draft for discussion, prepared with Project Equinox for PEAK6 Labs. For the build, docs/build-requirements.md is authoritative; this document restates its model as a proposal a neutral intermediary could adopt, and adds the standards context.

## 1. Motivation
The same real-world event trades on multiple venues under different names, structures and settlement rules. Identically worded questions can trade several points apart across venues [Predictions.io, cross-venue gaps]. Titles are not a safe join key: the same headline event can settle in opposite directions on two venues because their rules, interpretation standards and fallback settlement differ [DeFi Rate, settlement divergence] [Resolution trap, Fed cut example]. For a router, a false equivalence is not a data quality issue. It puts the user into a different bet.

## 2. Principle
Precision over recall. Two contracts are the same bet only when their payoffs are identical. Anything less is a different bet, however related [Semantic Non-Fungibility, arXiv 2601.01706].

## 3. The market card
A market is described by payoff fields, which decide the bet, and record fields, which only display it.

| Payoff field | Meaning |
|---|---|
| subject | What is measured (shared vocabulary) |
| cutoff_op | gt, gte, lt, lte |
| cutoff_value | Decimal string, no trailing zeros |
| cutoff_unit | percent, usd, count (shared vocabulary) |
| deadline | Moment the outcome is read, UTC |
| decider | Source that settles the market (shared vocabulary) |
| unclear_result | What is paid if the result is unclear, cancelled or split: yes_or_no_only, half, last_price, cancelled |

`unclear_result` exists because fallback settlement is part of the payoff. One venue settled an ambiguous outcome at the last traded price while another paid the full dollar on the same event [DeFi Rate, settlement divergence].

Unknown values are empty. Empty never equals anything.

## 4. Equivalence
| Result | Rule |
|---|---|
| same | All payoff fields non-empty and equal |
| opposite | All payoff fields non-empty and equal except cutoff_op, where the operators are exact complements (gt/lte, gte/lt) at the same value and unit |
| not_same | Every other compared pair; a finished answer with no confidence score |

Probabilistic record linkage scores pairs and leaves an uncertain middle band for review [Fellegi-Sunter, record linkage], and some products label near and related rows alongside exact ones [Orrery, exact/near/related]. This spec deliberately keeps only the exact class as routable, because a routing decision cannot act on "probably the same bet".

## 5. Routing
For a requested side on the reference venue: pick the equivalent position on the other venue (same side for `same`, other side for `opposite`), refuse if a price is missing or older than the age limit, compare cost as buy price plus fee when fees are known on both venues, choose the lower cost, and break exact ties with a fixed venue preference. This is a simplified form of the total consideration test used in best execution [FCA COBS 11.2A, total consideration] [MiFID II Art.27, best execution].

## 6. Decision record
Every comparison and every routing decision writes one append-only record with the result, the prices and times used, whether fees were applied, the action, and a plain-sentence reason naming the field or price that decided it.

## 7. Mapping to FINOS CDM (sketch)
The FINOS Common Domain Model provides an open representation of derivative products and lifecycle events [FINOS CDM, open derivatives model]. Event contracts are derivatives under CFTC jurisdiction [CFTC NPRM, June 2026]. A standards path should extend CDM rather than compete with it.

| Card field | CDM direction (to validate with maintainers) |
|---|---|
| subject, decider | Observable and its information source |
| cutoff_op, cutoff_value, cutoff_unit | Trigger condition on the observable |
| deadline | Observation and valuation date |
| unclear_result | Fallback and disruption provisions |
| Equivalence result | Not in CDM; proposed as a cross-venue reference-data extension |

This mapping is a sketch, not a verified modeling exercise.

## 8. Known gaps in this model
- Settlement mechanism is not a payoff field. With Polymarket US, both venues are CFTC-regulated exchanges that settle centrally [Polymarket US, CFTC venue], so the largest mechanism gap (oracle voting on Polymarket International [Polymarket, UMA resolution] [Event finality, arXiv 2610.00183]) does not apply. Differences in rule wording and fallback settlement remain, and `unclear_result` captures the fallback part.
- Vocabulary coverage is the limiting factor for recall. V1 covers Federal Reserve rate markets only.

## 9. Path to standardization
Venues have absorbed normalization layers rather than adopting neutral ones: Polymarket acquired the unified API startup Dome and retired its APIs [Dome acquisition, venue absorbs aggregator], and Kalshi pursued distribution through a Tradeweb data integration [Tradeweb-Kalshi, distribution]. Open unified SDKs already exist [pmxt, unified API], and brokers solve multi-venue access internally [IBKR, multi-venue access].

The party with the incentive to standardize equivalence is the neutral intermediary that routes across venues, which is Apex's position. A credible path: publish this card model and its vocabulary as an open reference, propose the payoff fields as a CDM extension through a FINOS working group sponsored by PEAK6 / Apex, and invite venues to publish settlement terms in this structured form. Researchers already expect resolution-rule conventions to standardize as venues mature [Underspecified rules, arXiv 2607.06166].

## Sources
Every bracketed tag resolves in docs/citations.md.
