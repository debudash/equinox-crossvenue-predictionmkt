# ADR-001: Equivalence is exact payoff-field equality

Status: Accepted. Requirements: FR-3, FR-7. Build reference: build-requirements "Equivalence".

## Context
PEAK6 asks candidates to define "equivalent" and justify it. Titles are not a safe join key. Identically worded questions trade apart across venues [Predictions.io, cross-venue gaps], and the same headline event has settled in opposite directions on Kalshi and Polymarket because rules, interpretation standards and fallback settlement differed [DeFi Rate, settlement divergence]. In Fed markets specifically, an off-cycle or emergency move can resolve one venue's contract YES and the other's NO [Resolution trap, Fed cut example]. For a router, a false match puts the user into a different bet.

## Decision
- A market is one card. Seven payoff fields decide the bet: subject, cutoff_op, cutoff_value, cutoff_unit, deadline, decider, unclear_result.
- `same`: all payoff fields non-empty and equal.
- `opposite`: all equal except cutoff operators that are exact complements (gt/lte, gte/lt) at the same value and unit.
- `not_same`: everything else, including any empty field, a stricter cutoff, an earlier deadline, or a different decider. It is a finished answer with no confidence score.
- `unclear_result` is a payoff field because fallback settlement changes what is paid. One venue settled an ambiguous outcome at last traded price while the other paid the full dollar [DeFi Rate, settlement divergence].
- Related contracts, including subsets and supersets, are `not_same` [Semantic Non-Fungibility, arXiv 2601.01706].

## Options considered
| Option | Why rejected |
|---|---|
| Title or text similarity | Fails exactly where it matters: same title, different settlement |
| Probabilistic linkage with a review band [Fellegi-Sunter, record linkage] | A router cannot act on "probably the same bet"; adds a review workflow the PRD did not ask for |
| Exact plus near and related labels [Orrery, exact/near/related] | Useful for research displays, but only exact is routable, so V1 keeps only exact |
| Language model verdict | Not reproducible, not auditable, and excluded by the build requirements |

## Consequences
- Recall is limited by vocabulary coverage and fetcher rules. That is the intended tradeoff and is measured in docs/metrics.md.
- Every `not_same` names the field that differed or was empty, which turns refusals into a map of what blocks normalization.
- Known gap: settlement mechanism (exchange determination vs oracle vote) is not a payoff field in V1 (docs/spec.md section 8).

## Sources
[Predictions.io, cross-venue gaps] [DeFi Rate, settlement divergence] [Resolution trap, Fed cut example] [Semantic Non-Fungibility, arXiv 2601.01706] [Fellegi-Sunter, record linkage] [Orrery, exact/near/related]
