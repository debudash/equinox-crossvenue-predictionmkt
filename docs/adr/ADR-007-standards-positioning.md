# ADR-007: Standards positioning - extend, do not compete

Status: Accepted. Business questions 1 and 2 in docs/brief.md.

## Context
Proposing a new industry standard is tempting. The evidence argues against a standalone one:
- Venues absorb normalization layers. Polymarket acquired the unified API startup Dome and retired its APIs [Dome acquisition, venue absorbs aggregator]. Kalshi pursued distribution through a Tradeweb data integration [Tradeweb-Kalshi, distribution].
- Open unified SDKs already exist [pmxt, unified API], and brokers solve multi-venue access internally [IBKR, multi-venue access].
- Academic work has already published human-validated cross-platform matched datasets [Semantic Non-Fungibility, arXiv 2601.01706].
- An open derivatives standard already exists [FINOS CDM, open derivatives model], and the regulator is still defining the event contract framework [CFTC NPRM, June 2026].

## Decision
- docs/spec.md is an RFC-style draft of the card model with a sketched mapping to FINOS CDM, not a competing schema.
- The README frames standardization as an opportunity for a neutral intermediary such as Apex to sponsor.
- No patent or proprietary claim. Record linkage has prior art back to 1969 [Fellegi-Sunter, record linkage].

## Consequences
- The contribution is a concrete, testable payoff-field model and a refusal log that shows which fields block equivalence in practice.

## Sources
[Dome acquisition, venue absorbs aggregator] [Tradeweb-Kalshi, distribution] [pmxt, unified API] [IBKR, multi-venue access] [Semantic Non-Fungibility, arXiv 2601.01706] [FINOS CDM, open derivatives model] [CFTC NPRM, June 2026] [Fellegi-Sunter, record linkage]
